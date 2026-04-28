package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type persistedState struct {
	ProfileDefaults  AgentProfile             `json:"profile_defaults,omitempty"`
	DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
	Agents           []persistedAgent         `json:"agents"`
	Workers          []legacyWorker           `json:"workers,omitempty"`
}

func (s persistedState) isObject() bool {
	return s.Agents != nil || s.Workers != nil || s.ProfileDefaults.Provider != "" || len(s.DetectionResults) > 0
}

type legacyWorker struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	ModelID     string    `json:"model_id,omitempty"`
}

type persistedAgent struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	Description      string                   `json:"description,omitempty"`
	Image            string                   `json:"image,omitempty"`
	BoxID            string                   `json:"box_id,omitempty"`
	Role             string                   `json:"role"`
	CreatedAt        time.Time                `json:"created_at"`
	Profile          string                   `json:"profile,omitempty"`
	Provider         string                   `json:"provider,omitempty"`
	ModelID          string                   `json:"model_id,omitempty"`
	ReasoningEffort  string                   `json:"reasoning_effort,omitempty"`
	AgentProfile     AgentProfile             `json:"agent_profile,omitempty"`
	ProfileComplete  bool                     `json:"profile_complete"`
	DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
}

func newPersistedAgent(a Agent) persistedAgent {
	return persistedAgent{
		ID:               a.ID,
		Name:             a.Name,
		Description:      a.Description,
		Image:            a.Image,
		BoxID:            a.BoxID,
		Role:             a.Role,
		CreatedAt:        a.CreatedAt,
		Profile:          a.Profile,
		Provider:         a.Provider,
		ModelID:          a.ModelID,
		ReasoningEffort:  a.ReasoningEffort,
		AgentProfile:     cloneProfile(a.AgentProfile),
		ProfileComplete:  a.ProfileComplete,
		DetectionResults: append([]ProfileDetectionResult(nil), a.DetectionResults...),
	}
}

func (a persistedAgent) toAgent() Agent {
	return Agent{
		ID:               a.ID,
		Name:             a.Name,
		Description:      a.Description,
		Image:            a.Image,
		BoxID:            a.BoxID,
		Role:             a.Role,
		CreatedAt:        a.CreatedAt,
		Profile:          a.Profile,
		Provider:         a.Provider,
		ModelID:          a.ModelID,
		ReasoningEffort:  a.ReasoningEffort,
		AgentProfile:     cloneProfile(a.AgentProfile),
		ProfileComplete:  a.ProfileComplete,
		DetectionResults: append([]ProfileDetectionResult(nil), a.DetectionResults...),
	}
}

func (w legacyWorker) toAgent() Agent {
	return Agent{
		ID:          w.ID,
		Name:        w.Name,
		Description: w.Description,
		Image:       "",
		Role:        RoleWorker,
		Status:      w.Status,
		CreatedAt:   w.CreatedAt,
		ModelID:     w.ModelID,
	}
}

func (s *Service) load() error {
	agents, err := s.readState()
	if err != nil {
		return err
	}
	for id, a := range agents {
		s.agents[id] = a
	}
	return nil
}

func (s *Service) Reload() error {
	agents, err := s.readState()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.agents = agents
	return nil
}

func (s *Service) readState() (map[string]Agent, error) {
	agents := make(map[string]Agent)
	if s.state == "" {
		return agents, nil
	}

	data, err := os.ReadFile(s.state)
	if err != nil {
		if os.IsNotExist(err) {
			return agents, nil
		}
		return nil, fmt.Errorf("read agent state: %w", err)
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err == nil && state.isObject() {
		if strings.TrimSpace(state.ProfileDefaults.Provider) != "" || strings.TrimSpace(state.ProfileDefaults.ModelID) != "" || strings.TrimSpace(state.ProfileDefaults.BaseURL) != "" {
			s.profileDefaults = normalizeProfile(state.ProfileDefaults, "", "")
		}
		s.detectionResults = append([]ProfileDetectionResult(nil), state.DetectionResults...)
		for _, a := range state.Agents {
			normalized := s.normalizeLoadedAgent(a.toAgent())
			agents[normalized.ID] = normalized
		}
		for _, w := range state.Workers {
			normalized := s.normalizeLoadedAgent(w.toAgent())
			agents[normalized.ID] = normalized
		}
		return agents, nil
	}

	var decoded []Agent
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("decode agent state: %w", err)
	}
	for _, a := range decoded {
		normalized := s.normalizeLoadedAgent(a)
		agents[normalized.ID] = normalized
	}
	return agents, nil
}

func (s *Service) saveLocked() error {
	if s.state == "" {
		return nil
	}

	data, err := json.MarshalIndent(persistedState{
		ProfileDefaults:  cloneProfile(s.profileDefaults),
		DetectionResults: append([]ProfileDetectionResult(nil), s.detectionResults...),
		Agents:           persistedAgentsFromMap(s.agents),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode agent state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.state), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if err := os.WriteFile(s.state, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write agent state: %w", err)
	}
	return nil
}

func (s *Service) normalizeLoadedAgent(a Agent) Agent {
	a = *cloneAgent(&a)
	a.Role = normalizeRole(a.Role)
	a.AgentProfile = normalizeProfile(a.AgentProfile, a.Name, a.Description)
	if !a.AgentProfile.ProfileComplete && (strings.TrimSpace(a.Provider) != "" || strings.TrimSpace(a.ModelID) != "") {
		legacyProfile := profileFromLegacy(a.Name, a.Description, a.Provider, a.ModelID, a.ReasoningEffort)
		if strings.TrimSpace(legacyProfile.BaseURL) == "" {
			legacyProfile.BaseURL = s.profileDefaults.BaseURL
		}
		if strings.TrimSpace(legacyProfile.APIKey) == "" {
			legacyProfile.APIKey = s.profileDefaults.APIKey
		}
		if len(legacyProfile.Headers) == 0 {
			legacyProfile.Headers = s.profileDefaults.Headers
		}
		a.AgentProfile = normalizeProfile(legacyProfile, a.Name, a.Description)
	}
	a.ProfileComplete = a.AgentProfile.ProfileComplete
	a.Provider = a.AgentProfile.Provider
	a.ModelID = a.AgentProfile.ModelID
	a.ReasoningEffort = a.AgentProfile.ReasoningEffort
	a.Profile = profileSelector(a.AgentProfile)
	if isManagerAgent(a) {
		a.ID = ManagerUserID
		a.Name = ManagerName
		a.Role = RoleManager
		if strings.TrimSpace(a.Image) == "" {
			a.Image = s.managerImage
		}
	}
	return a
}

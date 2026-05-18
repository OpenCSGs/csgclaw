package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/utils"
)

type persistedState struct {
	ProfileDefaults  AgentProfile             `json:"profile_defaults,omitempty"`
	DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
	Agents           []persistedAgent         `json:"agents"`
	Runtimes         []RuntimeRecord          `json:"runtimes,omitempty"`
	Workers          []legacyWorker           `json:"workers,omitempty"`
}

func (s persistedState) isObject() bool {
	return s.Agents != nil || s.Runtimes != nil || s.Workers != nil || s.ProfileDefaults.Provider != "" || len(s.DetectionResults) > 0
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
	RuntimeID        string                   `json:"runtime_id,omitempty"`
	RuntimeKind      string                   `json:"runtime_kind,omitempty"`
	Image            string                   `json:"image,omitempty"`
	BoxID            string                   `json:"box_id,omitempty"`
	RuntimeOptions   map[string]any           `json:"runtime_options,omitempty"`
	Role             string                   `json:"role"`
	Status           string                   `json:"status,omitempty"`
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
	ap := cloneProfile(a.AgentProfile)
	if strings.TrimSpace(ap.Name) == strings.TrimSpace(a.Name) {
		ap.Name = ""
	}
	if strings.TrimSpace(ap.Description) == strings.TrimSpace(a.Description) {
		ap.Description = ""
	}
	pol := agentruntime.RuntimeOptionsPolicyForKind(a.RuntimeKind)
	var topRX map[string]any
	if len(a.RuntimeOptions) > 0 {
		topRX = utils.CloneAnyMap(a.RuntimeOptions)
	}
	ap.BaseURL, ap.ModelID = pol.StripProfileLLMFields(a.RuntimeKind, ap.BaseURL, ap.ModelID)
	return persistedAgent{
		ID:               a.ID,
		Name:             a.Name,
		Description:      a.Description,
		RuntimeID:        a.RuntimeID,
		RuntimeKind:      a.RuntimeKind,
		Image:            a.Image,
		BoxID:            a.BoxID,
		RuntimeOptions:   topRX,
		Role:             a.Role,
		Status:           a.Status,
		CreatedAt:        a.CreatedAt,
		Profile:          a.Profile,
		AgentProfile:     ap,
		ProfileComplete:  a.ProfileComplete,
		DetectionResults: append([]ProfileDetectionResult(nil), a.DetectionResults...),
	}
}

func (a persistedAgent) toAgent() Agent {
	ap := cloneProfile(a.AgentProfile)
	rx := utils.CloneAnyMap(a.RuntimeOptions)
	if strings.TrimSpace(ap.Name) == "" {
		ap.Name = a.Name
	}
	if strings.TrimSpace(ap.Description) == "" {
		ap.Description = a.Description
	}
	// Backward compatibility for older persisted states: prefer agent_profile,
	// and only fall back to legacy top-level LLM fields while old snapshots may
	// still exist. Remove this fallback after the migration window ends.
	if strings.TrimSpace(ap.Provider) == "" {
		ap.Provider = strings.TrimSpace(a.Provider)
	}
	if strings.TrimSpace(ap.ModelID) == "" {
		ap.ModelID = strings.TrimSpace(a.ModelID)
	}
	if strings.TrimSpace(ap.ReasoningEffort) == "" {
		ap.ReasoningEffort = strings.TrimSpace(a.ReasoningEffort)
	}
	ag := Agent{
		ID:               a.ID,
		Name:             a.Name,
		Description:      a.Description,
		RuntimeID:        a.RuntimeID,
		RuntimeKind:      a.RuntimeKind,
		Image:            a.Image,
		BoxID:            a.BoxID,
		RuntimeOptions:   rx,
		Role:             a.Role,
		Status:           a.Status,
		CreatedAt:        a.CreatedAt,
		Profile:          a.Profile,
		AgentProfile:     ap,
		ProfileComplete:  a.ProfileComplete,
		DetectionResults: append([]ProfileDetectionResult(nil), a.DetectionResults...),
	}
	return ag
}

func (w legacyWorker) toAgent() Agent {
	return Agent{
		ID:          w.ID,
		Name:        w.Name,
		Description: w.Description,
		RuntimeID:   runtimeIDForAgentID(w.ID),
		RuntimeKind: RuntimeKindPicoClawSandbox,
		Image:       "",
		Role:        RoleWorker,
		Status:      w.Status,
		CreatedAt:   w.CreatedAt,
		AgentProfile: AgentProfile{
			ModelID: w.ModelID,
		},
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
		runtimes := make(map[string]RuntimeRecord, len(state.Runtimes))
		for _, rt := range state.Runtimes {
			normalized := normalizeRuntimeRecord(rt)
			if normalized.ID == "" {
				continue
			}
			if normalized.Kind == "" {
				return nil, fmt.Errorf("normalize persisted runtime %q: runtime kind is required", normalized.ID)
			}
			runtimes[normalized.ID] = normalized
		}
		for _, a := range state.Agents {
			normalized, err := s.normalizeLoadedAgent(a.toAgent())
			if err != nil {
				return nil, fmt.Errorf("normalize persisted agent %q: %w", strings.TrimSpace(a.ID), err)
			}
			if rt, ok := runtimes[normalized.RuntimeID]; ok && rt.Kind != "" {
				normalized.RuntimeKind = rt.Kind
			}
			agents[normalized.ID] = normalized
			if _, ok := runtimes[normalized.RuntimeID]; !ok {
				runtimes[normalized.RuntimeID] = runtimeRecordForAgent(normalized)
			}
		}
		for _, w := range state.Workers {
			normalized, err := s.normalizeLoadedAgent(w.toAgent())
			if err != nil {
				return nil, fmt.Errorf("normalize legacy worker %q: %w", strings.TrimSpace(w.ID), err)
			}
			if rt, ok := runtimes[normalized.RuntimeID]; ok && rt.Kind != "" {
				normalized.RuntimeKind = rt.Kind
			}
			agents[normalized.ID] = normalized
			if _, ok := runtimes[normalized.RuntimeID]; !ok {
				runtimes[normalized.RuntimeID] = runtimeRecordForAgent(normalized)
			}
		}
		s.runtimeRecords = runtimes
		return agents, nil
	}

	var decoded []Agent
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("decode agent state: %w", err)
	}
	for _, a := range decoded {
		normalized, err := s.normalizeLoadedAgent(a)
		if err != nil {
			return nil, fmt.Errorf("normalize state agent %q: %w", strings.TrimSpace(a.ID), err)
		}
		agents[normalized.ID] = normalized
	}
	runtimes := make(map[string]RuntimeRecord, len(agents))
	for _, a := range agents {
		runtimes[a.RuntimeID] = runtimeRecordForAgent(a)
	}
	s.runtimeRecords = runtimes
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
		Runtimes:         sortedRuntimeRecordsFromMap(s.runtimeRecords),
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

func (s *Service) normalizeLoadedAgent(a Agent) (Agent, error) {
	a = *cloneAgent(&a)
	a.ID = strings.TrimSpace(a.ID)
	if a.ID == "" {
		return Agent{}, fmt.Errorf("id is required")
	}
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return Agent{}, fmt.Errorf("name is required")
	}
	a.Role = normalizeRole(a.Role)
	a.RuntimeID = normalizeRuntimeID(a.RuntimeID, a.ID)
	if a.RuntimeKind == "" {
		return Agent{}, fmt.Errorf("runtime_kind is required")
	}
	if isManagerAgent(a) {
		switch {
		case a.ID != ManagerUserID:
			return Agent{}, fmt.Errorf("manager id must be %q", ManagerUserID)
		case a.Name != ManagerName:
			return Agent{}, fmt.Errorf("manager name must be %q", ManagerName)
		case a.Role != RoleManager:
			return Agent{}, fmt.Errorf("manager role must be %q", RoleManager)
		}
	}
	a.AgentProfile = normalizeProfile(a.AgentProfile, a.Name, a.Description)
	a.AgentProfile = normalizeProfileForAgentRuntime(a.AgentProfile, a.RuntimeOptions, a.Name, a.Description, a.RuntimeKind, nil)
	a.ProfileComplete = a.AgentProfile.ProfileComplete
	a.Profile = profileSelector(a.AgentProfile)
	if strings.TrimSpace(a.Status) == "" && strings.TrimSpace(a.BoxID) != "" {
		a.Status = string(sandbox.StateRunning)
	}
	return a, nil
}

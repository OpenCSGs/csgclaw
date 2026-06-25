package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"csgclaw/internal/config"
	"csgclaw/internal/localstore"
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

type rootAgentsState struct {
	ProfileDefaults  AgentProfile             `json:"profile_defaults,omitempty"`
	DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
	Items            []persistedAgent         `json:"items"`
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
	Instructions     string                   `json:"instructions,omitempty"`
	RuntimeID        string                   `json:"runtime_id,omitempty"`
	RuntimeKind      string                   `json:"runtime_kind,omitempty"`
	Image            string                   `json:"image,omitempty"`
	Avatar           string                   `json:"avatar,omitempty"`
	BoxID            string                   `json:"box_id,omitempty"`
	Runtime          *RuntimeRecord           `json:"runtime,omitempty"`
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
		Instructions:     a.Instructions,
		RuntimeID:        a.RuntimeID,
		RuntimeKind:      a.RuntimeKind,
		Image:            a.Image,
		Avatar:           a.Avatar,
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
	runtimeID := a.RuntimeID
	runtimeKind := a.RuntimeKind
	boxID := a.BoxID
	if a.Runtime != nil {
		rt := normalizeRuntimeRecord(*a.Runtime)
		if rt.ID != "" {
			runtimeID = rt.ID
		}
		if rt.Kind != "" {
			runtimeKind = rt.Kind
		}
		if rt.SandboxID != "" {
			boxID = rt.SandboxID
		}
	}
	ag := Agent{
		ID:               a.ID,
		Name:             a.Name,
		Description:      a.Description,
		Instructions:     a.Instructions,
		RuntimeID:        runtimeID,
		RuntimeKind:      runtimeKind,
		Image:            a.Image,
		Avatar:           a.Avatar,
		BoxID:            boxID,
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

	if root, ok, err := s.readRootAgentsState(); err != nil {
		return nil, err
	} else if ok {
		return s.agentsFromRootState(root)
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
			s.profileDefaults = s.catalogReferenceForLoadedProfile(normalizeProfile(state.ProfileDefaults, "", ""))
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
	if looksLikeJSONObject(data) {
		s.runtimeRecords = map[string]RuntimeRecord{}
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

func looksLikeJSONObject(data []byte) bool {
	var raw map[string]json.RawMessage
	return json.Unmarshal(data, &raw) == nil && raw != nil
}

func (s *Service) readRootAgentsState() (rootAgentsState, bool, error) {
	if !s.stateLooksLikeRootState() {
		return rootAgentsState{}, false, nil
	}
	var root rootAgentsState
	ok, err := localstore.ReadSection(s.state, "agents", &root)
	if err != nil {
		return rootAgentsState{}, false, err
	}
	return root, ok, nil
}

func (s *Service) agentsFromRootState(root rootAgentsState) (map[string]Agent, error) {
	agents := make(map[string]Agent)
	if strings.TrimSpace(root.ProfileDefaults.Provider) != "" ||
		strings.TrimSpace(root.ProfileDefaults.ModelProviderID) != "" ||
		strings.TrimSpace(root.ProfileDefaults.ModelID) != "" ||
		strings.TrimSpace(root.ProfileDefaults.BaseURL) != "" {
		s.profileDefaults = s.catalogReferenceForLoadedProfile(normalizeProfile(root.ProfileDefaults, "", ""))
	}
	s.detectionResults = append([]ProfileDetectionResult(nil), root.DetectionResults...)
	runtimes := make(map[string]RuntimeRecord, len(root.Items))
	for _, a := range root.Items {
		if a.Runtime != nil {
			rt := normalizeRuntimeRecord(*a.Runtime)
			if rt.ID != "" {
				runtimes[rt.ID] = rt
			}
		}
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
	s.runtimeRecords = runtimes
	return agents, nil
}

func (s *Service) saveLocked() error {
	if s.state == "" {
		return nil
	}

	if s.shouldWriteRootAgentsState() {
		return localstore.WriteSection(s.state, "agents", s.rootAgentsStateLocked())
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

func (s *Service) rootAgentsStateLocked() rootAgentsState {
	items := persistedAgentsFromMap(s.agents)
	for i := range items {
		runtimeID := strings.TrimSpace(items[i].RuntimeID)
		if runtimeID == "" {
			runtimeID = runtimeIDForAgentID(items[i].ID)
			items[i].RuntimeID = runtimeID
		}
		if rt, ok := s.runtimeRecords[runtimeID]; ok {
			normalized := normalizeRuntimeRecord(rt)
			items[i].Runtime = &normalized
		} else {
			normalized := runtimeRecordForAgent(items[i].toAgent())
			items[i].Runtime = &normalized
		}
	}
	return rootAgentsState{
		ProfileDefaults:  cloneProfile(s.profileDefaults),
		DetectionResults: append([]ProfileDetectionResult(nil), s.detectionResults...),
		Items:            items,
	}
}

func (s *Service) shouldWriteRootAgentsState() bool {
	if strings.TrimSpace(s.state) == "" {
		return false
	}
	if s.stateIsDefaultRootStatePath() {
		return true
	}
	return rootSectionExists(s.state, "agents")
}

func (s *Service) stateLooksLikeRootState() bool {
	if strings.TrimSpace(s.state) == "" {
		return false
	}
	return rootSectionExists(s.state, "agents")
}

func (s *Service) stateIsDefaultRootStatePath() bool {
	path := strings.TrimSpace(s.state)
	return filepath.Base(path) == config.StateFileName && filepath.Base(filepath.Dir(path)) == config.AppDirName
}

func rootSectionExists(path, section string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	value, ok := raw[section]
	if !ok || len(value) == 0 {
		return false
	}
	var probe map[string]any
	return json.Unmarshal(value, &probe) == nil
}

func (s *Service) normalizeLoadedAgent(a Agent) (Agent, error) {
	a = *cloneAgent(&a)
	a.ID = canonicalAgentID(a.ID)
	if a.ID == "" {
		return Agent{}, fmt.Errorf("id is required")
	}
	a.Name = strings.TrimSpace(a.Name)
	if a.Name == "" {
		return Agent{}, fmt.Errorf("name is required")
	}
	a.Role = normalizeRole(a.Role)
	if isManagerAgent(a) {
		a.ID = ManagerUserID
		if strings.TrimSpace(a.RuntimeID) == "" || strings.TrimSpace(a.RuntimeID) == "rt-u-manager" || strings.TrimSpace(a.RuntimeID) == "rt-manager" {
			a.RuntimeID = runtimeIDForAgentID(ManagerUserID)
		}
	}
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
	a.AgentProfile = s.catalogReferenceForLoadedProfile(a.AgentProfile)
	a.AgentProfile = normalizeProfileForAgentRuntime(a.AgentProfile, a.RuntimeOptions, a.Name, a.Description, a.RuntimeKind, nil)
	a.ProfileComplete = a.AgentProfile.ProfileComplete
	a.Profile = profileSelector(a.AgentProfile)
	if strings.TrimSpace(a.Status) == "" && strings.TrimSpace(a.BoxID) != "" {
		a.Status = string(sandbox.StateRunning)
	}
	return a, nil
}

func (s *Service) catalogReferenceForLoadedProfile(profile AgentProfile) AgentProfile {
	if s == nil {
		return profile
	}
	if migrated, ok := CatalogReferenceProfile(s.llm, profile); ok {
		return migrated
	}
	return profile
}

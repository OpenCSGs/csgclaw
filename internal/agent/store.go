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
	ProfileDefaults  AgentProfile             `json:"model_defaults,omitempty"`
	DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
	Items            []persistedAgent         `json:"items"`
}

func (s *rootAgentsState) UnmarshalJSON(data []byte) error {
	type rootAgentsStateJSON struct {
		ModelDefaults    AgentProfile             `json:"model_defaults,omitempty"`
		ProfileDefaults  AgentProfile             `json:"profile_defaults,omitempty"`
		DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
		Items            []persistedAgent         `json:"items"`
	}
	var decoded rootAgentsStateJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	defaults := decoded.ModelDefaults
	if profileEmpty(defaults) {
		defaults = decoded.ProfileDefaults
	}
	*s = rootAgentsState{
		ProfileDefaults:  cloneProfile(defaults),
		DetectionResults: append([]ProfileDetectionResult(nil), decoded.DetectionResults...),
		Items:            decoded.Items,
	}
	return nil
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
	RuntimeOptions   map[string]any           `json:"-"`
	Role             string                   `json:"role"`
	Status           string                   `json:"status,omitempty"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at,omitempty"`
	Profile          AgentProfile             `json:"model_config,omitempty"`
	ProfileSelector  string                   `json:"-"`
	Provider         string                   `json:"provider,omitempty"`
	ModelID          string                   `json:"model_id,omitempty"`
	ReasoningEffort  string                   `json:"reasoning_effort,omitempty"`
	AgentProfile     AgentProfile             `json:"agent_profile,omitempty"`
	ProfileComplete  bool                     `json:"profile_complete"`
	DetectionResults []ProfileDetectionResult `json:"detection_results,omitempty"`
}

func (a persistedAgent) MarshalJSON() ([]byte, error) {
	runtime := a.Runtime
	if runtime == nil {
		legacyRuntime := RuntimeRecord{
			ID:        normalizeRuntimeID(a.RuntimeID, a.ID),
			Kind:      strings.TrimSpace(a.RuntimeKind),
			State:     agentruntime.State(strings.TrimSpace(a.Status)),
			SandboxID: strings.TrimSpace(a.BoxID),
			Options:   utils.CloneAnyMap(a.RuntimeOptions),
			CreatedAt: a.CreatedAt,
		}
		runtime = compactPersistedRuntime(legacyRuntime, a.RuntimeOptions)
	}
	profile := a.Profile
	if profileEmpty(profile) {
		profile = compactPersistedProfile(a.AgentProfile)
	}
	out := map[string]any{
		"id":         a.ID,
		"name":       a.Name,
		"role":       a.Role,
		"created_at": a.CreatedAt,
	}
	if strings.TrimSpace(a.Description) != "" {
		out["description"] = a.Description
	}
	if strings.TrimSpace(a.Instructions) != "" {
		out["instructions"] = a.Instructions
	}
	if strings.TrimSpace(a.Image) != "" {
		out["image"] = a.Image
	}
	if !a.UpdatedAt.IsZero() {
		out["updated_at"] = a.UpdatedAt
	}
	if runtime != nil && !runtimeRecordEmpty(*runtime) {
		out["runtime"] = runtime
	}
	if !profileEmpty(profile) {
		out["model_config"] = profile
	}
	if len(a.DetectionResults) > 0 {
		out["detection_results"] = a.DetectionResults
	}
	return json.Marshal(out)
}

func (a *persistedAgent) UnmarshalJSON(data []byte) error {
	type persistedAgentAlias persistedAgent
	type persistedAgentJSON struct {
		persistedAgentAlias
		ModelConfig    json.RawMessage `json:"model_config"`
		Profile        json.RawMessage `json:"profile"`
		RuntimeOptions map[string]any  `json:"runtime_options"`
	}
	var decoded persistedAgentJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*a = persistedAgent(decoded.persistedAgentAlias)
	a.RuntimeOptions = utils.CloneAnyMap(decoded.RuntimeOptions)
	profilePayload := decoded.ModelConfig
	if len(profilePayload) == 0 || string(profilePayload) == "null" {
		profilePayload = decoded.Profile
	}
	if len(profilePayload) > 0 && string(profilePayload) != "null" {
		var profile AgentProfile
		if err := json.Unmarshal(profilePayload, &profile); err == nil {
			a.Profile = profile
		} else {
			var selector string
			if err := json.Unmarshal(profilePayload, &selector); err == nil {
				a.ProfileSelector = strings.TrimSpace(selector)
			} else {
				return fmt.Errorf("decode agent profile: %w", err)
			}
		}
	}
	if a.Runtime != nil && len(a.Runtime.Options) > 0 && len(a.RuntimeOptions) == 0 {
		a.RuntimeOptions = utils.CloneAnyMap(a.Runtime.Options)
	}
	return nil
}

func runtimeRecordEmpty(rt RuntimeRecord) bool {
	return strings.TrimSpace(rt.ID) == "" &&
		strings.TrimSpace(rt.Kind) == "" &&
		strings.TrimSpace(string(rt.State)) == "" &&
		len(rt.AgentIDs) == 0 &&
		strings.TrimSpace(rt.SandboxID) == "" &&
		len(rt.Options) == 0 &&
		rt.CreatedAt.IsZero()
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
	ap = compactPersistedProfile(ap)
	updatedAt := a.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = a.CreatedAt.UTC()
	}
	return persistedAgent{
		ID:               a.ID,
		Name:             a.Name,
		Description:      a.Description,
		Instructions:     a.Instructions,
		Image:            a.Image,
		Runtime:          compactPersistedRuntime(runtimeRecordForAgent(a), topRX),
		RuntimeOptions:   topRX,
		Role:             a.Role,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        updatedAt,
		Profile:          ap,
		DetectionResults: append([]ProfileDetectionResult(nil), a.DetectionResults...),
	}
}

func (a persistedAgent) toAgent() Agent {
	ap := cloneProfile(a.Profile)
	if profileEmpty(ap) {
		ap = cloneProfile(a.AgentProfile)
	}
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
	status := a.Status
	if a.Runtime != nil {
		rt := normalizeRuntimeRecord(*a.Runtime)
		if rt.ID != "" {
			runtimeID = rt.ID
		}
		if runtimeID == "" {
			runtimeID = runtimeIDForAgentID(a.ID)
		}
		if rt.Kind != "" {
			runtimeKind = rt.Kind
		}
		if rt.SandboxID != "" {
			boxID = rt.SandboxID
		}
		if strings.TrimSpace(status) == "" && rt.State != "" {
			status = string(rt.State)
		}
		if len(rx) == 0 && len(rt.Options) > 0 {
			rx = utils.CloneAnyMap(rt.Options)
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
		Status:           status,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
		Profile:          a.ProfileSelector,
		AgentProfile:     ap,
		ProfileComplete:  a.ProfileComplete,
		DetectionResults: append([]ProfileDetectionResult(nil), a.DetectionResults...),
	}
	return ag
}

func profileEmpty(profile AgentProfile) bool {
	return strings.TrimSpace(profile.Name) == "" &&
		strings.TrimSpace(profile.Description) == "" &&
		strings.TrimSpace(profile.Provider) == "" &&
		strings.TrimSpace(profile.ModelProviderID) == "" &&
		strings.TrimSpace(profile.BaseURL) == "" &&
		strings.TrimSpace(profile.APIKey) == "" &&
		len(profile.Headers) == 0 &&
		strings.TrimSpace(profile.ModelID) == "" &&
		strings.TrimSpace(profile.ReasoningEffort) == "" &&
		!profile.EnableFastMode &&
		len(profile.RequestOptions) == 0 &&
		len(profile.Env) == 0 &&
		!profile.EnvRestartRequired &&
		!profile.ImageUpgradeRequired
}

func compactPersistedProfile(profile AgentProfile) AgentProfile {
	out := cloneProfile(profile)
	if strings.TrimSpace(out.ModelProviderID) == "" {
		switch normalizeProfileProvider(out.Provider) {
		case ProviderCSGHubLite:
			out.ModelProviderID = ModelProviderIDCSGHubLite
		case ProviderCSGHub, ProviderOpenCSG:
			out.ModelProviderID = ModelProviderIDOpenCSG
		case ProviderCodex:
			out.ModelProviderID = ModelProviderIDCodex
		case ProviderClaudeCode:
			out.ModelProviderID = ModelProviderIDClaude
		}
	}
	out.Provider = ""
	out.ProfileComplete = false
	return out
}

func compactPersistedProfileDefaults(profile AgentProfile) AgentProfile {
	out := compactPersistedProfile(profile)
	out.Name = ""
	out.Description = ""
	return out
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
			raw := a.toAgent()
			if strings.TrimSpace(raw.RuntimeID) == "" {
				raw.RuntimeID = runtimeIDForAgentID(raw.ID)
			}
			if rt, ok := runtimes[raw.RuntimeID]; ok {
				raw = applyRuntimeRecordToAgent(raw, rt)
			}
			normalized, err := s.normalizeLoadedAgent(raw)
			if err != nil {
				return nil, fmt.Errorf("normalize persisted agent %q: %w", strings.TrimSpace(a.ID), err)
			}
			agents[normalized.ID] = normalized
			if _, ok := runtimes[normalized.RuntimeID]; !ok {
				runtimes[normalized.RuntimeID] = runtimeRecordForAgent(normalized)
			}
		}
		for _, w := range state.Workers {
			raw := w.toAgent()
			if strings.TrimSpace(raw.RuntimeID) == "" {
				raw.RuntimeID = runtimeIDForAgentID(raw.ID)
			}
			if rt, ok := runtimes[raw.RuntimeID]; ok {
				raw = applyRuntimeRecordToAgent(raw, rt)
			}
			normalized, err := s.normalizeLoadedAgent(raw)
			if err != nil {
				return nil, fmt.Errorf("normalize legacy worker %q: %w", strings.TrimSpace(w.ID), err)
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
		raw := a.toAgent()
		if strings.TrimSpace(raw.RuntimeID) == "" {
			raw.RuntimeID = runtimeIDForAgentID(raw.ID)
		}
		if rt, ok := runtimes[raw.RuntimeID]; ok {
			raw = applyRuntimeRecordToAgent(raw, rt)
		}
		normalized, err := s.normalizeLoadedAgent(raw)
		if err != nil {
			return nil, fmt.Errorf("normalize persisted agent %q: %w", strings.TrimSpace(a.ID), err)
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
		runtimeID := runtimeIDForAgentID(items[i].ID)
		if runtimeID == "" {
			runtimeID = strings.TrimSpace(items[i].RuntimeID)
		}
		if rt, ok := s.runtimeRecords[runtimeID]; ok {
			normalized := normalizeRuntimeRecord(rt)
			items[i].Runtime = compactPersistedRuntime(normalized, items[i].RuntimeOptions)
		} else {
			normalized := runtimeRecordForAgent(items[i].toAgent())
			items[i].Runtime = compactPersistedRuntime(normalized, items[i].RuntimeOptions)
		}
	}
	return rootAgentsState{
		ProfileDefaults:  compactPersistedProfileDefaults(s.profileDefaults),
		DetectionResults: append([]ProfileDetectionResult(nil), s.detectionResults...),
		Items:            items,
	}
}

func compactPersistedRuntime(rt RuntimeRecord, options map[string]any) *RuntimeRecord {
	rt = normalizeRuntimeRecord(rt)
	if len(options) > 0 {
		rt.Options = utils.CloneAnyMap(options)
	}
	rt.ID = ""
	rt.AgentIDs = nil
	rt.CreatedAt = time.Time{}
	return &rt
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

func applyRuntimeRecordToAgent(a Agent, rt RuntimeRecord) Agent {
	rt = normalizeRuntimeRecord(rt)
	if strings.TrimSpace(rt.ID) != "" {
		a.RuntimeID = rt.ID
	}
	if strings.TrimSpace(a.RuntimeID) == "" {
		a.RuntimeID = runtimeIDForAgentID(a.ID)
	}
	if strings.TrimSpace(rt.Kind) != "" {
		a.RuntimeKind = rt.Kind
	}
	if strings.TrimSpace(rt.SandboxID) != "" {
		a.BoxID = rt.SandboxID
	}
	if strings.TrimSpace(a.Status) == "" && rt.State != "" {
		a.Status = string(rt.State)
	}
	if len(a.RuntimeOptions) == 0 && len(rt.Options) > 0 {
		a.RuntimeOptions = utils.CloneAnyMap(rt.Options)
	}
	return a
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
		a.Role = RoleManager
		if strings.TrimSpace(a.RuntimeID) == "" || strings.TrimSpace(a.RuntimeID) == "rt-u-manager" || strings.TrimSpace(a.RuntimeID) == "rt-manager" {
			a.RuntimeID = runtimeIDForAgentID(ManagerUserID)
		}
	}
	a.RuntimeID = normalizeRuntimeID(a.RuntimeID, a.ID)
	if strings.TrimSpace(a.RuntimeID) == "" {
		a.RuntimeID = runtimeIDForAgentID(a.ID)
	}
	if a.RuntimeKind == "" {
		return Agent{}, fmt.Errorf("runtime_kind is required")
	}
	if isManagerAgent(a) {
		switch {
		case a.ID != ManagerUserID:
			return Agent{}, fmt.Errorf("manager id must be %q", ManagerUserID)
		case a.Role != RoleManager:
			return Agent{}, fmt.Errorf("manager role must be %q", RoleManager)
		}
	}
	a.AgentProfile = normalizeProfile(a.AgentProfile, a.Name, a.Description)
	a.AgentProfile = s.catalogReferenceForLoadedProfile(a.AgentProfile)
	a.AgentProfile = normalizeProfileForAgentRuntime(a.AgentProfile, a.RuntimeOptions, a.Name, a.Description, a.RuntimeKind, nil)
	a.ProfileComplete = a.AgentProfile.ProfileComplete
	a.Profile = profileSelector(a.AgentProfile)
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = a.CreatedAt
	}
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

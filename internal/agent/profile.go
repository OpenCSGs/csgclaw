package agent

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"csgclaw/internal/cliproxy"
	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"
)

const (
	ProviderAPI        = "api"
	ProviderCSGHubLite = "csghub_lite"
	ProviderCodex      = "codex"
	ProviderClaudeCode = "claude_code"

	DefaultReasoningEffort = "medium"
)

var (
	defaultCSGHubLiteBaseURL = modelprovider.CSGHubLiteDefaultBaseURL
	defaultCSGHubLiteAPIKey  = modelprovider.CSGHubLiteDefaultAPIKey
	listCLIProxyModels       = func(ctx context.Context, provider string) ([]string, error) {
		return cliproxy.Default().ListModels(ctx, provider)
	}
	listCLIProxyModelChoices = func(ctx context.Context, provider string) ([]string, error) {
		return cliproxy.Default().ListModelChoices(ctx, provider)
	}
)

type AgentProfile struct {
	Name               string            `json:"name,omitempty"`
	Description        string            `json:"description,omitempty"`
	Provider           string            `json:"provider,omitempty"`
	BaseURL            string            `json:"base_url,omitempty"`
	APIKey             string            `json:"api_key,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	ModelID            string            `json:"model_id,omitempty"`
	ReasoningEffort    string            `json:"reasoning_effort,omitempty"`
	EnableFastMode     bool              `json:"enable_fast_mode,omitempty"`
	RequestOptions     map[string]any    `json:"request_options,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	ProfileComplete    bool              `json:"profile_complete"`
	EnvRestartRequired bool              `json:"env_restart_required,omitempty"`
}

type AgentProfileView struct {
	Name               string                   `json:"name,omitempty"`
	Description        string                   `json:"description,omitempty"`
	Provider           string                   `json:"provider,omitempty"`
	BaseURL            string                   `json:"base_url,omitempty"`
	APIKeySet          bool                     `json:"api_key_set,omitempty"`
	Headers            map[string]string        `json:"headers,omitempty"`
	ModelID            string                   `json:"model_id,omitempty"`
	ReasoningEffort    string                   `json:"reasoning_effort,omitempty"`
	EnableFastMode     bool                     `json:"enable_fast_mode"`
	RequestOptions     map[string]any           `json:"request_options,omitempty"`
	Env                map[string]string        `json:"env,omitempty"`
	ProfileComplete    bool                     `json:"profile_complete"`
	EnvRestartRequired bool                     `json:"env_restart_required,omitempty"`
	DetectionResults   []ProfileDetectionResult `json:"detection_results,omitempty"`
}

type ProfileDetectionResult struct {
	Provider string `json:"provider"`
	Status   string `json:"status"`
	ModelID  string `json:"model_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

type ProfileModelRequest struct {
	Provider string            `json:"provider"`
	BaseURL  string            `json:"base_url,omitempty"`
	APIKey   string            `json:"api_key,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

type ProfileModelsResponse struct {
	Provider string   `json:"provider"`
	Models   []string `json:"models"`
}

func normalizeProfile(profile AgentProfile, fallbackName, fallbackDescription string) AgentProfile {
	out := profile
	out.Name = strings.TrimSpace(out.Name)
	if out.Name == "" {
		out.Name = strings.TrimSpace(fallbackName)
	}
	out.Description = strings.TrimSpace(out.Description)
	if out.Description == "" {
		out.Description = strings.TrimSpace(fallbackDescription)
	}
	out.Provider = normalizeProfileProvider(out.Provider)
	out.BaseURL = strings.TrimRight(strings.TrimSpace(out.BaseURL), "/")
	out.APIKey = strings.TrimSpace(out.APIKey)
	out.ModelID = strings.TrimSpace(out.ModelID)
	out.ReasoningEffort = strings.ToLower(strings.TrimSpace(out.ReasoningEffort))
	if out.ReasoningEffort == "" {
		out.ReasoningEffort = DefaultReasoningEffort
	}
	out.Headers = normalizeStringMap(out.Headers)
	out.Env = normalizeStringMap(out.Env)
	out.RequestOptions = normalizeRequestOptions(out.RequestOptions)
	out.ProfileComplete = profileIsComplete(out)
	return out
}

func normalizeProfileProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", ProviderCSGHubLite, "csghub-lite":
		return ProviderCSGHubLite
	case ProviderAPI, "llm-api", "openai", "openai_compatible":
		return ProviderAPI
	case ProviderCodex:
		return ProviderCodex
	case ProviderClaudeCode, "claude-code":
		return ProviderClaudeCode
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func profileIsComplete(profile AgentProfile) bool {
	if strings.TrimSpace(profile.Name) == "" {
		return false
	}
	switch normalizeProfileProvider(profile.Provider) {
	case ProviderAPI:
		return strings.TrimSpace(profile.BaseURL) != "" && strings.TrimSpace(profile.APIKey) != "" && strings.TrimSpace(profile.ModelID) != ""
	case ProviderCSGHubLite, ProviderCodex, ProviderClaudeCode:
		return strings.TrimSpace(profile.ModelID) != ""
	default:
		return false
	}
}

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRequestOptions(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || strings.EqualFold(key, "model") {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneProfile(profile AgentProfile) AgentProfile {
	out := profile
	if len(profile.Headers) > 0 {
		out.Headers = make(map[string]string, len(profile.Headers))
		for key, value := range profile.Headers {
			out.Headers[key] = value
		}
	}
	if len(profile.Env) > 0 {
		out.Env = make(map[string]string, len(profile.Env))
		for key, value := range profile.Env {
			out.Env[key] = value
		}
	}
	if len(profile.RequestOptions) > 0 {
		out.RequestOptions = make(map[string]any, len(profile.RequestOptions))
		for key, value := range profile.RequestOptions {
			out.RequestOptions[key] = value
		}
	}
	return out
}

func profileView(profile AgentProfile, detection []ProfileDetectionResult) AgentProfileView {
	profile = cloneProfile(profile)
	return AgentProfileView{
		Name:               profile.Name,
		Description:        profile.Description,
		Provider:           profile.Provider,
		BaseURL:            profile.BaseURL,
		APIKeySet:          strings.TrimSpace(profile.APIKey) != "",
		Headers:            profile.Headers,
		ModelID:            profile.ModelID,
		ReasoningEffort:    profile.ReasoningEffort,
		EnableFastMode:     profile.EnableFastMode,
		RequestOptions:     profile.RequestOptions,
		Env:                profile.Env,
		ProfileComplete:    profile.ProfileComplete,
		EnvRestartRequired: profile.EnvRestartRequired,
		DetectionResults:   append([]ProfileDetectionResult(nil), detection...),
	}
}

func RedactedProfileView(profile AgentProfile, detection []ProfileDetectionResult) AgentProfileView {
	return profileView(profile, detection)
}

func profileSelector(profile AgentProfile) string {
	provider := normalizeProfileProvider(profile.Provider)
	modelID := strings.TrimSpace(profile.ModelID)
	if provider == "" || modelID == "" {
		return ""
	}
	return provider + "." + modelID
}

func profileFromLegacy(name, description, provider, modelID, reasoning string) AgentProfile {
	return normalizeProfile(AgentProfile{
		Name:            name,
		Description:     description,
		Provider:        provider,
		ModelID:         modelID,
		ReasoningEffort: reasoning,
	}, name, description)
}

func profileFromConfigModel(name, description string, model config.ModelConfig) AgentProfile {
	model = model.Resolved()
	return normalizeProfile(AgentProfile{
		Name:            name,
		Description:     description,
		Provider:        model.Provider,
		BaseURL:         model.BaseURL,
		APIKey:          model.APIKey,
		ModelID:         model.ModelID,
		ReasoningEffort: model.ReasoningEffort,
	}, name, description)
}

func profileBaseURL(profile AgentProfile) string {
	switch normalizeProfileProvider(profile.Provider) {
	case ProviderAPI:
		return strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
	case ProviderCSGHubLite:
		return defaultCSGHubLiteBaseURL
	case ProviderCodex, ProviderClaudeCode:
		return ""
	default:
		return strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
	}
}

func ProfileBaseURL(profile AgentProfile) string {
	return profileBaseURL(profile)
}

func profileAPIKey(profile AgentProfile) string {
	switch normalizeProfileProvider(profile.Provider) {
	case ProviderAPI:
		return strings.TrimSpace(profile.APIKey)
	case ProviderCSGHubLite:
		return defaultCSGHubLiteAPIKey
	default:
		return strings.TrimSpace(profile.APIKey)
	}
}

func ProfileAPIKey(profile AgentProfile) string {
	return profileAPIKey(profile)
}

func (s *Service) DetectDefaultProfile(ctx context.Context) (AgentProfile, []ProfileDetectionResult) {
	if s != nil {
		s.mu.RLock()
		defaultProfile := cloneProfile(s.profileDefaults)
		s.mu.RUnlock()
		defaultProfile = normalizeProfile(defaultProfile, ManagerName, "")
		if defaultProfile.ProfileComplete {
			if models, err := ListModelsForProfile(ctx, defaultProfile); err == nil && len(models) > 0 {
				if defaultProfile.ModelID == "" {
					defaultProfile.ModelID = models[0]
				}
				defaultProfile.ProfileComplete = true
				return defaultProfile, []ProfileDetectionResult{{
					Provider: defaultProfile.Provider,
					Status:   "ok",
					ModelID:  defaultProfile.ModelID,
				}}
			}
		}
	}

	probes := []AgentProfile{
		{Name: ManagerName, Provider: ProviderCSGHubLite},
		{Name: ManagerName, Provider: ProviderCodex},
		{Name: ManagerName, Provider: ProviderClaudeCode},
	}
	results := make([]ProfileDetectionResult, 0, len(probes))
	for _, probe := range probes {
		probe = normalizeProfile(probe, ManagerName, "")
		models, err := ListModelsForProfile(ctx, probe)
		if err != nil {
			results = append(results, ProfileDetectionResult{
				Provider: probe.Provider,
				Status:   "failed",
				Error:    err.Error(),
			})
			continue
		}
		if len(models) == 0 {
			results = append(results, ProfileDetectionResult{
				Provider: probe.Provider,
				Status:   "failed",
				Error:    "no models returned",
			})
			continue
		}
		probe.ModelID = models[0]
		probe.ProfileComplete = true
		results = append(results, ProfileDetectionResult{
			Provider: probe.Provider,
			Status:   "ok",
			ModelID:  probe.ModelID,
		})
		return probe, results
	}
	return normalizeProfile(AgentProfile{Name: ManagerName, Provider: ProviderCSGHubLite}, ManagerName, ""), results
}

func ListModelsForProfile(ctx context.Context, profile AgentProfile) ([]string, error) {
	profile = normalizeProfile(profile, profile.Name, profile.Description)
	switch profile.Provider {
	case ProviderCodex, ProviderClaudeCode:
		models, err := listCLIProxyModels(ctx, profile.Provider)
		if err != nil {
			return nil, err
		}
		return sortModelIDs(models), nil
	}
	baseURL := profileBaseURL(profile)
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	models, err := modelprovider.ListOpenAIModelsWithClient(ctx, client, baseURL, profileAPIKey(profile), profile.Headers)
	if err != nil {
		return nil, err
	}
	return sortModelIDs(models), nil
}

func sortModelIDs(models []string) []string {
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, model)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return modelLess(out[i], out[j])
	})
	return out
}

func modelLess(left, right string) bool {
	leftFamily, leftNumbers, leftVariant := modelSortParts(left)
	rightFamily, rightNumbers, rightVariant := modelSortParts(right)
	if leftFamily != rightFamily {
		return leftFamily < rightFamily
	}
	maxLen := len(leftNumbers)
	if len(rightNumbers) > maxLen {
		maxLen = len(rightNumbers)
	}
	for idx := 0; idx < maxLen; idx++ {
		leftNum, rightNum := 0, 0
		if idx < len(leftNumbers) {
			leftNum = leftNumbers[idx]
		}
		if idx < len(rightNumbers) {
			rightNum = rightNumbers[idx]
		}
		if leftNum != rightNum {
			return leftNum > rightNum
		}
	}
	if leftVariant != rightVariant {
		return leftVariant > rightVariant
	}
	return strings.ToLower(left) < strings.ToLower(right)
}

func modelSortParts(model string) (int, []int, int) {
	name := strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndexAny(name, "/:"); idx >= 0 && idx < len(name)-1 {
		name = name[idx+1:]
	}
	family := 90
	switch {
	case strings.HasPrefix(name, "gpt-"):
		family = 0
	case strings.HasPrefix(name, "o") && len(name) > 1 && name[1] >= '0' && name[1] <= '9':
		family = 1
	case strings.HasPrefix(name, "claude-"):
		family = 2
	case strings.HasPrefix(name, "qwen"):
		family = 3
	}
	return family, numericParts(name), modelVariantRank(name)
}

func numericParts(value string) []int {
	parts := make([]int, 0, 3)
	for idx := 0; idx < len(value); {
		if value[idx] < '0' || value[idx] > '9' {
			idx++
			continue
		}
		next := idx
		num := 0
		for next < len(value) && value[next] >= '0' && value[next] <= '9' {
			num = num*10 + int(value[next]-'0')
			next++
		}
		parts = append(parts, num)
		idx = next
	}
	return parts
}

func modelVariantRank(name string) int {
	switch {
	case strings.Contains(name, "pro"), strings.Contains(name, "max"):
		return 70
	case strings.Contains(name, "mini"):
		return 40
	case strings.Contains(name, "nano"), strings.Contains(name, "small"):
		return 30
	default:
		return 50
	}
}

func profilesEqualEnv(a, b AgentProfile) bool {
	if len(a.Env) != len(b.Env) {
		return false
	}
	for key, value := range a.Env {
		if b.Env[key] != value {
			return false
		}
	}
	return true
}

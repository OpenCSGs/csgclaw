package agent

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"
)

const (
	ModelProviderIDCSGHubLite = "csghub-lite"
	ModelProviderIDCodex      = ProviderCodex
	ModelProviderIDClaude     = ProviderClaudeCode

	ModelProviderKindCSGHubLite       = ProviderCSGHubLite
	ModelProviderKindCodex            = ProviderCodex
	ModelProviderKindClaudeCode       = ProviderClaudeCode
	ModelProviderKindOpenAICompatible = "openai_compatible"

	ModelProviderStatusUnknown   = "unknown"
	ModelProviderStatusConnected = "connected"
	ModelProviderStatusFailed    = "failed"
)

var builtinModelProviderIDs = []string{
	ModelProviderIDCSGHubLite,
	ModelProviderIDCodex,
	ModelProviderIDClaude,
}

type ModelProviderCatalog struct {
	DefaultSelector string                 `json:"default_selector,omitempty"`
	Providers       []ModelProviderSummary `json:"providers"`
}

type ModelProviderSummary struct {
	ID              string            `json:"id"`
	Kind            string            `json:"kind"`
	DisplayName     string            `json:"display_name"`
	Builtin         bool              `json:"builtin"`
	BaseURL         string            `json:"base_url,omitempty"`
	APIKey          string            `json:"api_key,omitempty"`
	APIKeySet       bool              `json:"api_key_set"`
	APIKeyPreview   string            `json:"api_key_preview,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Models          []string          `json:"models"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
	Status          string            `json:"status"`
	Message         string            `json:"message,omitempty"`
	LastCheckedAt   string            `json:"last_checked_at,omitempty"`
}

type ModelProviderCheckResult struct {
	ID            string   `json:"id"`
	Status        string   `json:"status"`
	Message       string   `json:"message,omitempty"`
	Models        []string `json:"models"`
	LastCheckedAt string   `json:"last_checked_at"`
}

type ModelProviderCheckInput struct {
	ID      string
	BaseURL string
	APIKey  string
	Headers map[string]string
	Models  []string
}

type ModelProviderCheckFunc func(context.Context, ModelProviderCheckInput) ModelProviderCheckResult

func IsBuiltinModelProviderID(id string) bool {
	switch NormalizeModelProviderID(id) {
	case ModelProviderIDCSGHubLite, ModelProviderIDCodex, ModelProviderIDClaude:
		return true
	default:
		return false
	}
}

func NormalizeModelProviderID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	switch id {
	case "", "legacy-inline":
		return ""
	case "csghub_lite", "csghublite", modelprovider.CSGHubLiteProviderName:
		return ModelProviderIDCSGHubLite
	case "claude-code", "claude":
		return ModelProviderIDClaude
	case ProviderCodex:
		return ModelProviderIDCodex
	case ProviderClaudeCode:
		return ModelProviderIDClaude
	}

	var b strings.Builder
	lastDash := false
	for _, r := range id {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == '_' || r == '-':
			if !lastDash && b.Len() > 0 {
				b.WriteRune(r)
				lastDash = r == '-'
			}
		case unicode.IsSpace(r), r == '.', r == '/', r == ':':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-_")
	switch out {
	case "csghub-lite":
		return ModelProviderIDCSGHubLite
	case "claude-code", "claude_code":
		return ModelProviderIDClaude
	default:
		return out
	}
}

func ProfileProviderForModelProviderID(id string) string {
	switch NormalizeModelProviderID(id) {
	case ModelProviderIDCSGHubLite:
		return ProviderCSGHubLite
	case ModelProviderIDCodex:
		return ProviderCodex
	case ModelProviderIDClaude:
		return ProviderClaudeCode
	default:
		return ProviderAPI
	}
}

func splitModelProviderSelector(selector string) (string, string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", "", false
	}
	provider, modelID, ok := strings.Cut(selector, ".")
	if !ok {
		provider, modelID, ok = strings.Cut(selector, ":")
	}
	provider = NormalizeModelProviderID(provider)
	modelID = strings.TrimSpace(modelID)
	return provider, modelID, ok && provider != "" && modelID != ""
}

func SelectorUsesModelProvider(selector, id string) bool {
	providerID, _, ok := splitModelProviderSelector(selector)
	return ok && NormalizeModelProviderID(providerID) == NormalizeModelProviderID(id)
}

func ModelProviderCatalogFromLLM(llm config.LLMConfig) ModelProviderCatalog {
	cfg := llm.Normalized()
	providers := make([]ModelProviderSummary, 0, len(cfg.Providers)+len(builtinModelProviderIDs))
	seen := make(map[string]struct{}, len(cfg.Providers)+len(builtinModelProviderIDs))
	for _, id := range builtinModelProviderIDs {
		providers = append(providers, builtinModelProviderSummary(id, cfg.Providers[id]))
		seen[id] = struct{}{}
	}
	customIDs := make([]string, 0, len(cfg.Providers))
	for id := range cfg.Providers {
		normalizedID := NormalizeModelProviderID(id)
		if normalizedID == "" {
			continue
		}
		if _, ok := seen[normalizedID]; ok || IsBuiltinModelProviderID(normalizedID) {
			continue
		}
		customIDs = append(customIDs, normalizedID)
	}
	sort.Strings(customIDs)
	for _, id := range customIDs {
		providers = append(providers, customProviderSummary(id, cfg.Providers[id]))
	}
	return ModelProviderCatalog{
		DefaultSelector: cfg.DefaultSelector(),
		Providers:       providers,
	}
}

func builtinModelProviderSummary(id string, provider config.ProviderConfig) ModelProviderSummary {
	provider = provider.Resolved()
	summary := ModelProviderSummary{
		ID:            id,
		Builtin:       true,
		Models:        append([]string(nil), provider.Models...),
		Status:        ModelProviderStatusUnknown,
		APIKeySet:     true,
		APIKeyPreview: apiKeyPreview(defaultCSGHubLiteAPIKey),
	}
	switch id {
	case ModelProviderIDCSGHubLite:
		summary.Kind = ModelProviderKindCSGHubLite
		summary.DisplayName = "CSGHub Lite"
		summary.BaseURL = strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
		if summary.BaseURL == "" {
			summary.BaseURL = defaultCSGHubLiteBaseURL
		}
	case ModelProviderIDCodex:
		summary.Kind = ModelProviderKindCodex
		summary.DisplayName = "Codex"
		summary.BaseURL = ""
		summary.APIKeySet = false
		summary.APIKeyPreview = ""
	case ModelProviderIDClaude:
		summary.Kind = ModelProviderKindClaudeCode
		summary.DisplayName = "Claude Code"
		summary.BaseURL = ""
		summary.APIKeySet = false
		summary.APIKeyPreview = ""
	default:
		summary.Kind = ModelProviderKindOpenAICompatible
		summary.DisplayName = id
	}
	if provider.DisplayName != "" && !IsBuiltinModelProviderID(id) {
		summary.DisplayName = provider.DisplayName
	}
	if provider.APIKey != "" {
		summary.APIKeySet = true
		summary.APIKeyPreview = apiKeyPreview(provider.APIKey)
	}
	summary.Headers = cloneStringMap(provider.Headers)
	summary.ReasoningEffort = provider.ReasoningEffort
	applyProviderCheckMetadata(&summary, provider)
	return summary
}

func customProviderSummary(id string, provider config.ProviderConfig) ModelProviderSummary {
	provider = provider.Resolved()
	displayName := provider.DisplayName
	if displayName == "" {
		displayName = id
	}
	return ModelProviderSummary{
		ID:              id,
		Kind:            ModelProviderKindOpenAICompatible,
		DisplayName:     displayName,
		Builtin:         false,
		BaseURL:         provider.BaseURL,
		APIKeySet:       strings.TrimSpace(provider.APIKey) != "",
		APIKeyPreview:   apiKeyPreview(provider.APIKey),
		Headers:         cloneStringMap(provider.Headers),
		Models:          append([]string(nil), provider.Models...),
		ReasoningEffort: provider.ReasoningEffort,
		Status:          providerStatusOrUnknown(provider.Status),
		Message:         provider.Message,
		LastCheckedAt:   provider.LastCheckedAt,
	}
}

func CheckModelProvider(ctx context.Context, input ModelProviderCheckInput) ModelProviderCheckResult {
	id := NormalizeModelProviderID(input.ID)
	result := ModelProviderCheckResult{
		ID:            id,
		Status:        ModelProviderStatusFailed,
		LastCheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	var (
		models []string
		err    error
	)
	switch id {
	case ModelProviderIDCodex:
		models, err = listCLIProxyModelChoices(ctx, ProviderCodex)
	case ModelProviderIDClaude:
		models, err = listCLIProxyModelChoices(ctx, ProviderClaudeCode)
	default:
		baseURL := strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
		apiKey := strings.TrimSpace(input.APIKey)
		if id == ModelProviderIDCSGHubLite {
			if baseURL == "" {
				baseURL = defaultCSGHubLiteBaseURL
			}
			if apiKey == "" {
				apiKey = defaultCSGHubLiteAPIKey
			}
		}
		models, err = modelprovider.ListOpenAIModelsWithClient(ctx, &http.Client{Timeout: 3 * time.Second}, baseURL, apiKey, input.Headers)
	}
	if err != nil {
		result.Message = conciseProviderError(err)
		return result
	}
	result.Status = ModelProviderStatusConnected
	result.Models = sortModelIDs(models)
	result.Message = "connected"
	return result
}

func RefreshModelProviderCatalog(ctx context.Context, llm config.LLMConfig, check ModelProviderCheckFunc) (config.LLMConfig, []ModelProviderCheckResult, bool) {
	if check == nil {
		check = CheckModelProvider
	}
	cfg := llm.Normalized()
	catalog := ModelProviderCatalogFromLLM(cfg)
	results := make([]ModelProviderCheckResult, 0, len(catalog.Providers))
	changed := false
	for _, provider := range catalog.Providers {
		id := NormalizeModelProviderID(provider.ID)
		if id == "" {
			continue
		}
		existing := cfg.Providers[id].Resolved()
		result := check(ctx, ModelProviderCheckInput{
			ID:      id,
			BaseURL: existing.BaseURL,
			APIKey:  existing.APIKey,
			Headers: existing.Headers,
			Models:  existing.Models,
		})
		if result.ID == "" {
			result.ID = id
		}
		results = append(results, result)
		var didChange bool
		cfg, didChange = ApplyModelProviderCheckResult(cfg, id, result)
		if didChange {
			changed = true
		}
	}
	return cfg, results, changed
}

func ApplyModelProviderCheckResult(llm config.LLMConfig, id string, result ModelProviderCheckResult) (config.LLMConfig, bool) {
	id = NormalizeModelProviderID(id)
	if id == "" {
		return llm, false
	}
	cfg := llm.Normalized()
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}
	_, exists := cfg.Providers[id]
	if !exists && !IsBuiltinModelProviderID(id) {
		return llm, false
	}
	existing := cfg.Providers[id].Resolved()
	updated := providerConfigWithCheckResult(id, existing, result)
	changed := !providerConfigsEqual(existing, updated)
	modelsChanged := !sameStringSlice(existing.Models, updated.Models)
	if modelsChanged {
		delete(cfg.Profiles, id)
	}
	if changed {
		cfg.Providers[id] = updated
	}
	return cfg, changed
}

func providerConfigWithCheckResult(id string, existing config.ProviderConfig, result ModelProviderCheckResult) config.ProviderConfig {
	out := existing.Resolved()
	out.Status = providerStatusOrUnknown(result.Status)
	out.Message = strings.TrimSpace(result.Message)
	out.LastCheckedAt = strings.TrimSpace(result.LastCheckedAt)
	if result.Status == ModelProviderStatusConnected && len(result.Models) > 0 {
		out.Models = append([]string(nil), result.Models...)
	}
	if result.Status == ModelProviderStatusConnected && NormalizeModelProviderID(id) == ModelProviderIDCSGHubLite {
		if strings.TrimSpace(out.BaseURL) == "" {
			out.BaseURL = defaultCSGHubLiteBaseURL
		}
		if strings.TrimSpace(out.APIKey) == "" {
			out.APIKey = defaultCSGHubLiteAPIKey
		}
	}
	return out.Resolved()
}

func providerConfigsEqual(left, right config.ProviderConfig) bool {
	left = left.Resolved()
	right = right.Resolved()
	if left.DisplayName != right.DisplayName ||
		left.BaseURL != right.BaseURL ||
		left.APIKey != right.APIKey ||
		left.ReasoningEffort != right.ReasoningEffort ||
		left.Status != right.Status ||
		left.Message != right.Message ||
		left.LastCheckedAt != right.LastCheckedAt ||
		!stringMapsEqual(left.Headers, right.Headers) ||
		len(left.Models) != len(right.Models) {
		return false
	}
	for i := range left.Models {
		if left.Models[i] != right.Models[i] {
			return false
		}
	}
	return true
}

func applyProviderCheckMetadata(summary *ModelProviderSummary, provider config.ProviderConfig) {
	status := providerStatusOrUnknown(provider.Status)
	if status != ModelProviderStatusUnknown {
		summary.Status = status
	}
	if provider.Message != "" {
		summary.Message = provider.Message
	}
	if provider.LastCheckedAt != "" {
		summary.LastCheckedAt = provider.LastCheckedAt
	}
}

func providerStatusOrUnknown(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case ModelProviderStatusConnected:
		return ModelProviderStatusConnected
	case ModelProviderStatusFailed:
		return ModelProviderStatusFailed
	default:
		return ModelProviderStatusUnknown
	}
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func conciseProviderError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "check failed"
	}
	if len(msg) > 240 {
		return msg[:240]
	}
	return msg
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func ModelProviderConfigForProfile(llm config.LLMConfig, profile AgentProfile) (config.ProviderConfig, bool) {
	id := NormalizeModelProviderID(profile.ModelProviderID)
	if id == "" {
		return config.ProviderConfig{}, false
	}
	cfg := llm.Normalized()
	provider, ok := cfg.Providers[id]
	if ok {
		return provider.Resolved(), true
	}
	switch id {
	case ModelProviderIDCSGHubLite:
		return config.ProviderConfig{
			BaseURL: defaultCSGHubLiteBaseURL,
			APIKey:  defaultCSGHubLiteAPIKey,
		}.Resolved(), true
	case ModelProviderIDCodex, ModelProviderIDClaude:
		return config.ProviderConfig{}.Resolved(), true
	default:
		return config.ProviderConfig{}, false
	}
}

func CatalogProviderModelConfig(llm config.LLMConfig, selector string) (AgentProfile, bool) {
	providerID, modelID, ok := splitModelProviderSelector(selector)
	if !ok {
		return AgentProfile{}, false
	}
	profile := AgentProfile{
		Provider:        ProfileProviderForModelProviderID(providerID),
		ModelProviderID: providerID,
		ModelID:         modelID,
	}
	if provider, found := ModelProviderConfigForProfile(llm, profile); found {
		profile.ReasoningEffort = provider.ReasoningEffort
	}
	profile.ProfileComplete = profileIsComplete(profile)
	return profile, true
}

func CatalogReferenceProfile(llm config.LLMConfig, profile AgentProfile) (AgentProfile, bool) {
	profile = normalizeProfile(profile, profile.Name, profile.Description)
	if strings.TrimSpace(profile.ModelID) == "" {
		return profile, false
	}
	if id := NormalizeModelProviderID(profile.ModelProviderID); id != "" {
		return stripCatalogProviderCredentials(profile, id), true
	}
	id, ok := catalogProviderIDForProfile(llm, profile)
	if !ok {
		return profile, false
	}
	return stripCatalogProviderCredentials(profile, id), true
}

func catalogProviderIDForProfile(llm config.LLMConfig, profile AgentProfile) (string, bool) {
	switch normalizeProfileProvider(profile.Provider) {
	case ProviderCSGHubLite:
		return catalogBuiltinProviderIDForProfile(llm, ModelProviderIDCSGHubLite, profile)
	case ProviderCodex:
		return ModelProviderIDCodex, strings.TrimSpace(profile.ModelID) != ""
	case ProviderClaudeCode:
		return ModelProviderIDClaude, strings.TrimSpace(profile.ModelID) != ""
	case ProviderAPI:
		return catalogAPIProviderIDForProfile(llm, profile)
	default:
		return "", false
	}
}

func catalogBuiltinProviderIDForProfile(llm config.LLMConfig, id string, profile AgentProfile) (string, bool) {
	if strings.TrimSpace(profile.ModelID) == "" {
		return "", false
	}
	provider, found := ModelProviderConfigForProfile(llm, AgentProfile{ModelProviderID: id})
	if !found {
		return "", false
	}
	profileBaseURL := strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
	profileAPIKey := strings.TrimSpace(profile.APIKey)
	if profileBaseURL != "" && profileBaseURL != strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/") {
		return "", false
	}
	if profileAPIKey != "" && profileAPIKey != strings.TrimSpace(provider.APIKey) {
		return "", false
	}
	if len(profile.Headers) > 0 && !stringMapsEqual(profile.Headers, provider.Headers) {
		return "", false
	}
	return id, true
}

func catalogAPIProviderIDForProfile(llm config.LLMConfig, profile AgentProfile) (string, bool) {
	cfg := llm.Normalized()
	profileBaseURL := strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
	if profileBaseURL == "" {
		return "", false
	}
	for _, id := range sortedProviderIDs(cfg.Providers) {
		normalizedID := NormalizeModelProviderID(id)
		if normalizedID == "" || IsBuiltinModelProviderID(normalizedID) {
			continue
		}
		provider := cfg.Providers[id].Resolved()
		if !providerMatchesInlineAPIProfile(provider, profile) {
			continue
		}
		return normalizedID, true
	}
	return "", false
}

func providerMatchesInlineAPIProfile(provider config.ProviderConfig, profile AgentProfile) bool {
	provider = provider.Resolved()
	if !stringSliceContains(provider.Models, strings.TrimSpace(profile.ModelID)) {
		return false
	}
	if strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/") != strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/") {
		return false
	}
	if apiKey := strings.TrimSpace(profile.APIKey); apiKey != "" && strings.TrimSpace(provider.APIKey) != apiKey {
		return false
	}
	if len(profile.Headers) > 0 && !stringMapsEqual(profile.Headers, provider.Headers) {
		return false
	}
	return true
}

func stripCatalogProviderCredentials(profile AgentProfile, providerID string) AgentProfile {
	out := cloneProfile(profile)
	out.ModelProviderID = NormalizeModelProviderID(providerID)
	out.Provider = ProfileProviderForModelProviderID(out.ModelProviderID)
	out.BaseURL = ""
	out.APIKey = ""
	out.Headers = nil
	out.ProfileComplete = profileIsComplete(out)
	return out
}

func sortedProviderIDs(providers map[string]config.ProviderConfig) []string {
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func stringMapsEqual(left, right map[string]string) bool {
	left = normalizeStringMap(left)
	right = normalizeStringMap(right)
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func stringSliceContains(values []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func EnsureUniqueCustomModelProviderID(llm config.LLMConfig, requested, displayName string) (string, error) {
	base := NormalizeModelProviderID(requested)
	if base == "" {
		base = NormalizeModelProviderID(displayName)
	}
	if base == "" {
		base = "openai"
	}
	if IsBuiltinModelProviderID(base) {
		return "", fmt.Errorf("provider id %q is reserved", base)
	}
	normalized := llm.Normalized()
	if _, exists := normalized.Providers[base]; !exists {
		return base, nil
	}
	for i := 2; i < 1000; i++ {
		next := fmt.Sprintf("%s-%d", base, i)
		if _, exists := normalized.Providers[next]; !exists {
			return next, nil
		}
	}
	return "", fmt.Errorf("no available provider id for %q", base)
}

func ValidateUniqueModelProviderDisplayName(llm config.LLMConfig, providerID, displayName string) error {
	providerID = NormalizeModelProviderID(providerID)
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return fmt.Errorf("display name is required")
	}
	want := strings.ToLower(displayName)
	for _, provider := range ModelProviderCatalogFromLLM(llm).Providers {
		if NormalizeModelProviderID(provider.ID) == providerID {
			continue
		}
		if strings.ToLower(strings.TrimSpace(provider.DisplayName)) == want {
			return fmt.Errorf("display name %q is already used", displayName)
		}
	}
	return nil
}

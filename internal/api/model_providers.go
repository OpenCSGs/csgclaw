package api

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
)

var appCheckModelProvider = agent.CheckModelProvider

type modelProviderRequest struct {
	ID              string            `json:"id,omitempty"`
	DisplayName     string            `json:"display_name,omitempty"`
	Preset          string            `json:"preset,omitempty"`
	BaseURL         string            `json:"base_url,omitempty"`
	APIKey          string            `json:"api_key,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Models          []string          `json:"models,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
}

func (h *Handler) handleModelProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listModelProviders(w, r)
	case http.MethodPost:
		h.createModelProvider(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleModelProviderByID(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPut:
		h.updateModelProvider(w, r)
	case http.MethodDelete:
		h.deleteModelProvider(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) listModelProviders(w http.ResponseWriter, r *http.Request) {
	cfg, _, err := h.loadBootstrapConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, agent.ModelProviderCatalogFromLLM(cfg.Models))
}

func (h *Handler) createModelProvider(w http.ResponseWriter, r *http.Request) {
	var req modelProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	cfg, path, err := h.loadBootstrapConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		req.DisplayName = defaultModelProviderDisplayName(req.Preset)
	}
	if err := agent.ValidateUniqueModelProviderDisplayName(cfg.Models, "", req.DisplayName); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	id, err := agent.EnsureUniqueCustomModelProviderID(cfg.Models, req.ID, req.DisplayName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg.Models = cfg.Models.Normalized()
	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]config.ProviderConfig)
	}
	cfg.Models.Providers[id] = providerConfigFromRequest(config.ProviderConfig{}, req, false).Resolved()
	delete(cfg.Models.Profiles, id)
	if err := h.saveModelProvidersConfig(path, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	summary := agent.ModelProviderCatalogFromLLM(cfg.Models)
	writeJSON(w, http.StatusCreated, findProviderSummary(summary, id))
}

func (h *Handler) updateModelProvider(w http.ResponseWriter, r *http.Request) {
	id := agent.NormalizeModelProviderID(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	var req modelProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	cfg, path, err := h.loadBootstrapConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.Models = cfg.Models.Normalized()
	if cfg.Models.Providers == nil {
		cfg.Models.Providers = make(map[string]config.ProviderConfig)
	}
	existing, exists := cfg.Models.Providers[id]
	if !exists && !agent.IsBuiltinModelProviderID(id) {
		http.Error(w, fmt.Sprintf("model provider %q not found", id), http.StatusNotFound)
		return
	}
	if agent.IsBuiltinModelProviderID(id) {
		req.DisplayName = ""
	} else if strings.TrimSpace(req.DisplayName) != "" {
		if err := agent.ValidateUniqueModelProviderDisplayName(cfg.Models, id, req.DisplayName); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	updatedProvider := providerConfigFromRequest(existing, req, true).Resolved()
	providerInUse := h.modelProviderInUse(cfg.Models, id)
	if providerInUse && modelProviderTransportChanged(existing, updatedProvider) {
		result := appCheckModelProvider(r.Context(), agent.ModelProviderCheckInput{
			ID:      id,
			BaseURL: updatedProvider.BaseURL,
			APIKey:  updatedProvider.APIKey,
			Headers: updatedProvider.Headers,
			Models:  updatedProvider.Models,
		})
		if result.Status != agent.ModelProviderStatusConnected {
			message := strings.TrimSpace(result.Message)
			if message == "" {
				message = "connection check failed"
			}
			http.Error(w, "model provider is in use and the updated connection is unavailable: "+message, http.StatusBadRequest)
			return
		}
		cfg.Models.Providers[id] = updatedProvider
		if checked, changed := agent.ApplyModelProviderCheckResult(cfg.Models, id, result); changed {
			cfg.Models = checked
		}
	} else {
		cfg.Models.Providers[id] = updatedProvider
	}
	delete(cfg.Models.Profiles, id)
	if err := h.saveModelProvidersConfig(path, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	summary := agent.ModelProviderCatalogFromLLM(cfg.Models)
	writeJSON(w, http.StatusOK, findProviderSummary(summary, id))
}

func modelProviderTransportChanged(existing, updated config.ProviderConfig) bool {
	existing = existing.Resolved()
	updated = updated.Resolved()
	return existing.BaseURL != updated.BaseURL ||
		existing.APIKey != updated.APIKey ||
		!maps.Equal(existing.Headers, updated.Headers)
}

func (h *Handler) deleteModelProvider(w http.ResponseWriter, r *http.Request) {
	id := agent.NormalizeModelProviderID(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	if agent.IsBuiltinModelProviderID(id) {
		http.Error(w, "builtin model providers cannot be deleted", http.StatusBadRequest)
		return
	}
	cfg, path, err := h.loadBootstrapConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.Models = cfg.Models.Normalized()
	if _, ok := cfg.Models.Providers[id]; !ok {
		http.NotFound(w, r)
		return
	}
	if h.modelProviderInUse(cfg.Models, id) {
		http.Error(w, "model provider is in use", http.StatusConflict)
		return
	}
	delete(cfg.Models.Providers, id)
	delete(cfg.Models.Profiles, id)
	if err := h.saveModelProvidersConfig(path, cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) checkModelProvider(w http.ResponseWriter, r *http.Request) {
	id := agent.NormalizeModelProviderID(chi.URLParam(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	var req modelProviderRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
	}
	cfg, path, err := h.loadBootstrapConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.Models = cfg.Models.Normalized()
	existing := cfg.Models.Providers[id]
	merged := providerConfigFromRequest(existing, req, true).Resolved()
	result := appCheckModelProvider(r.Context(), agent.ModelProviderCheckInput{
		ID:      id,
		BaseURL: merged.BaseURL,
		APIKey:  merged.APIKey,
		Headers: merged.Headers,
		Models:  merged.Models,
	})
	if updated, changed := agent.ApplyModelProviderCheckResult(cfg.Models, id, result); changed {
		cfg.Models = updated
		if err := h.saveModelProvidersConfig(path, cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) refreshOpenCSGModelProvider(ctx context.Context) error {
	if h == nil || strings.TrimSpace(h.configPath) == "" {
		return nil
	}
	cfg, path, err := h.loadBootstrapConfig()
	if err != nil {
		return err
	}
	cfg.Models = cfg.Models.Normalized()
	existing := cfg.Models.Providers[agent.ModelProviderIDOpenCSG].Resolved()
	result := appCheckModelProvider(ctx, agent.ModelProviderCheckInput{
		ID:      agent.ModelProviderIDOpenCSG,
		BaseURL: existing.BaseURL,
		APIKey:  existing.APIKey,
		Headers: existing.Headers,
		Models:  existing.Models,
	})
	models, cleared := agent.ClearModelProviderCachedState(cfg.Models, agent.ModelProviderIDOpenCSG)
	models, applied := agent.ApplyModelProviderCheckResult(models, agent.ModelProviderIDOpenCSG, result)
	if !cleared && !applied {
		return nil
	}
	cfg.Models = models
	return h.saveModelProvidersConfig(path, cfg)
}

func providerConfigFromRequest(existing config.ProviderConfig, req modelProviderRequest, preserveSecret bool) config.ProviderConfig {
	out := existing.Resolved()
	if strings.TrimSpace(req.DisplayName) != "" {
		out.DisplayName = strings.TrimSpace(req.DisplayName)
	}
	if strings.TrimSpace(req.Preset) != "" {
		out.Preset = strings.ToLower(strings.TrimSpace(req.Preset))
	}
	if strings.TrimSpace(req.BaseURL) != "" {
		out.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	}
	if strings.TrimSpace(req.APIKey) != "" || !preserveSecret {
		out.APIKey = strings.TrimSpace(req.APIKey)
	}
	if req.Headers != nil {
		out.Headers = req.Headers
	}
	if req.Models != nil {
		out.Models = append([]string(nil), req.Models...)
	}
	if strings.TrimSpace(req.ReasoningEffort) != "" {
		out.ReasoningEffort = strings.TrimSpace(req.ReasoningEffort)
	}
	return out
}

func defaultModelProviderDisplayName(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "zhipu":
		return "Zhipu API"
	case "deepseek":
		return "DeepSeek API"
	case "custom":
		return "Custom API"
	default:
		return "OpenAI API"
	}
}

func (h *Handler) saveModelProvidersConfig(path string, cfg config.Config) error {
	if err := cfg.Save(path); err != nil {
		return err
	}
	if h != nil && h.svc != nil {
		h.svc.SetLLMConfig(cfg.Models)
	}
	return nil
}

func findProviderSummary(catalog agent.ModelProviderCatalog, id string) agent.ModelProviderSummary {
	id = agent.NormalizeModelProviderID(id)
	for _, provider := range catalog.Providers {
		if agent.NormalizeModelProviderID(provider.ID) == id {
			return provider
		}
	}
	return agent.ModelProviderSummary{ID: id, Status: agent.ModelProviderStatusUnknown}
}

func (h *Handler) modelProviderInUse(llm config.LLMConfig, id string) bool {
	id = agent.NormalizeModelProviderID(id)
	if id == "" {
		return false
	}
	cfg := llm.Normalized()
	if agent.SelectorUsesModelProvider(cfg.Default, id) ||
		agent.SelectorUsesModelProvider(cfg.DefaultProfile, id) ||
		agent.SelectorUsesModelProvider(cfg.DefaultSelector(), id) {
		return true
	}
	if h == nil || h.svc == nil {
		return false
	}
	for _, item := range h.svc.List() {
		if agent.NormalizeModelProviderID(item.AgentProfile.ModelProviderID) == id {
			return true
		}
		if agent.SelectorUsesModelProvider(item.Profile, id) {
			return true
		}
	}
	return false
}

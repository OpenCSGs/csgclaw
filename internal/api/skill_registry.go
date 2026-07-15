package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	skillapi "csgclaw/internal/skill"
	skilllocal "csgclaw/internal/skill/local"
)

const registrySkillPageSize = 16
const registrySkillMaxPageSize = 50
const registrySkillMaxPage = 100

type registrySkillSearchResponse struct {
	HasMore  bool                    `json:"has_more"`
	Items    []skillapi.SearchResult `json:"items"`
	NextPage *int                    `json:"next_page,omitempty"`
	Page     int                     `json:"page"`
	Per      int                     `json:"per"`
	Total    *int                    `json:"total,omitempty"`
}

type registrySkillInstallRequest struct {
	Slug       string `json:"slug,omitempty"`
	Version    string `json:"version,omitempty"`
	Registry   string `json:"registry,omitempty"`
	Replace    bool   `json:"replace,omitempty"`
	RemotePath string `json:"remote_path,omitempty"`
	Ref        string `json:"ref,omitempty"`
}

func (h *Handler) handleSkillRegistrySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "search query is required", http.StatusBadRequest)
		return
	}
	page := boundedPositiveQueryInt(r, "page", 1, registrySkillMaxPage)
	per := boundedPositiveQueryInt(r, "per", registrySkillPageSize, registrySkillMaxPageSize)
	limit := page*per + 1
	svc, err := h.skillRegistryServiceForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := svc.Search(r.Context(), query, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	start, end, hasMore := registrySkillPageBounds(len(items), page, per)
	var nextPage *int
	if hasMore {
		next := page + 1
		nextPage = &next
	}
	writeJSON(w, http.StatusOK, registrySkillSearchResponse{
		HasMore:  hasMore,
		Items:    items[start:end],
		NextPage: nextPage,
		Page:     page,
		Per:      per,
	})
}

func (h *Handler) handleSkillRegistryInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req registrySkillInstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = strings.TrimSpace(req.RemotePath)
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		version = strings.TrimSpace(req.Ref)
	}
	registry, err := skillapi.ParseRegistry(req.Registry)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	root, err := skilllocal.SkillsRoot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	svc, err := h.skillRegistryServiceForRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := svc.Install(r.Context(), slug, version, registry, root, req.Replace)
	if err != nil {
		writeRegistrySkillInstallError(w, err)
		return
	}
	item, err := localSkillSummary(root, result.Slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) skillRegistryServiceForRequest(r *http.Request) (*skillapi.Service, error) {
	cfg, _, err := h.loadBootstrapConfig()
	if err != nil {
		return nil, err
	}
	skillCfg := skillConfigForEnvironment(cfg.Skill, h.currentOpenCSGEnvironment(r))
	return skillapi.NewService(skillCfg, nil), nil
}

func localSkillSummary(root, name string) (skilllocal.SkillSummary, error) {
	items, err := skilllocal.List(root)
	if err != nil {
		return skilllocal.SkillSummary{}, err
	}
	for _, item := range items {
		if item.Name == name {
			return item, nil
		}
	}
	return skilllocal.SkillSummary{Name: name}, nil
}

func writeRegistrySkillInstallError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, skillapi.ErrSkillDirExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case skillapi.IsNotFound(err):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}

func registrySkillPageBounds(total, page, per int) (int, int, bool) {
	start := (page - 1) * per
	if start > total {
		start = total
	}
	end := start + per
	if end > total {
		end = total
	}
	return start, end, total > page*per
}

func positiveQueryInt(r *http.Request, key string, fallback int) int {
	return boundedPositiveQueryInt(r, key, fallback, 0)
}

func boundedPositiveQueryInt(r *http.Request, key string, fallback, maxValue int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	if maxValue > 0 && parsed > maxValue {
		return maxValue
	}
	return parsed
}

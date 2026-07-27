package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	skilllocal "csgclaw/internal/skill/local"
	skillremote "csgclaw/internal/skill/remote"
)

const (
	remoteSkillsDefaultPage = 1
	remoteSkillsDefaultPer  = 16
	remoteSkillsMaxPer      = 100
)

var errOfficialHubURLNotConfigured = errors.New("official Hub URL is not configured")

type remoteSkillsListResponse struct {
	Items    []remoteSkillSummary `json:"items"`
	NextPage *int                 `json:"next_page,omitempty"`
	Page     int                  `json:"page"`
	Per      int                  `json:"per"`
	Total    *int                 `json:"total,omitempty"`
}

type remoteSkillSummary struct {
	Description string `json:"description,omitempty"`
	Name        string `json:"name"`
	Readonly    bool   `json:"readonly"`
	RemotePath  string `json:"remote_path"`
	RemoteRef   string `json:"remote_ref,omitempty"`
	RemoteURL   string `json:"remote_url,omitempty"`
	Source      string `json:"source"`
}

type skillInstallRequest struct {
	RemotePath string `json:"remote_path"`
	Ref        string `json:"ref,omitempty"`
	Replace    bool   `json:"replace,omitempty"`
}

func (h *Handler) handleRemoteSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page, per, err := remoteSkillsPageOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	baseURL, err := h.remoteSkillsHubBaseURL(r)
	if err != nil {
		writeRemoteSkillsHubError(w, err)
		return
	}
	list, err := skillremote.ListAgenticHubSkills(r.Context(), baseURL, skillremote.AgenticHubSkillListOptions{
		Page:   page,
		Per:    per,
		Search: r.URL.Query().Get("search"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	items := make([]remoteSkillSummary, 0, len(list.Items))
	for _, item := range list.Items {
		remoteURL, err := skillremote.AgenticHubSkillWebURL(baseURL, item.RemotePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		items = append(items, remoteSkillSummary{
			Description: item.Description,
			Name:        item.Name,
			Readonly:    true,
			RemotePath:  item.RemotePath,
			RemoteRef:   item.Ref,
			RemoteURL:   remoteURL,
			Source:      "official",
		})
	}
	writeJSON(w, http.StatusOK, remoteSkillsListResponse{
		Items:    items,
		NextPage: nextRemoteSkillsPage(page, per, list.Total, list.RecordCount),
		Page:     page,
		Per:      per,
		Total:    list.Total,
	})
}

func (h *Handler) handleSkillInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req skillInstallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	remotePath, ref, err := skillremote.NormalizeAgenticHubSkillRequest(req.RemotePath, req.Ref)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	root, err := skilllocal.SkillsRoot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	baseURL, err := h.remoteSkillsHubBaseURL(r)
	if err != nil {
		writeRemoteSkillsHubError(w, err)
		return
	}
	archive, err := skillremote.FetchAgenticHubSkillArchive(r.Context(), baseURL, remotePath, ref)
	if err != nil {
		if skillremote.IsInvalidAgenticHubRequest(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	item, err := skilllocal.InstallArchiveWithOptions(
		root,
		skillremote.AgenticHubSkillArchiveName(remotePath),
		archive,
		skilllocal.InstallArchiveOptions{Replace: req.Replace},
	)
	if err != nil {
		writeSkillInstallError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) remoteSkillsHubBaseURL(r *http.Request) (string, error) {
	cfg, _, err := h.loadBootstrapConfig()
	if err != nil {
		return "", err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(h.officialHubBaseURLForRequest(r, cfg)), "/")
	if baseURL == "" {
		return "", errOfficialHubURLNotConfigured
	}
	return baseURL, nil
}

func writeRemoteSkillsHubError(w http.ResponseWriter, err error) {
	if errors.Is(err, errOfficialHubURLNotConfigured) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func remoteSkillsPageOptions(r *http.Request) (int, int, error) {
	page, err := remoteSkillsQueryInt(r, "page", remoteSkillsDefaultPage, 0)
	if err != nil {
		return 0, 0, err
	}
	per, err := remoteSkillsQueryInt(r, "per", remoteSkillsDefaultPer, remoteSkillsMaxPer)
	if err != nil {
		return 0, 0, err
	}
	return page, per, nil
}

func remoteSkillsQueryInt(r *http.Request, key string, fallback, maximum int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || (maximum > 0 && value > maximum) {
		if maximum > 0 {
			return 0, fmt.Errorf("%s must be an integer between 1 and %d", key, maximum)
		}
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func nextRemoteSkillsPage(page, per int, total *int, recordCount int) *int {
	if total != nil {
		if page*per >= *total {
			return nil
		}
	} else if recordCount < per {
		return nil
	}
	next := page + 1
	return &next
}

func writeSkillInstallError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, skilllocal.ErrSkillAlreadyExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, skilllocal.ErrSkillArchiveEmpty),
		errors.Is(err, skilllocal.ErrSkillArchiveUnsafe),
		errors.Is(err, skilllocal.ErrSkillArchiveInvalid),
		errors.Is(err, skilllocal.ErrSKILLMDMissing):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	skillinstall "csgclaw/internal/skill/installstate"
	skilllocal "csgclaw/internal/skill/local"
	skillremote "csgclaw/internal/skill/remote"
)

const (
	remoteSkillsDefaultPage = 1
	remoteSkillsDefaultPer  = 16
	remoteSkillsMaxPer      = 100
)

var errOfficialHubURLNotConfigured = errors.New("official Hub URL is not configured")

var remoteSkillsHubAccessToken = currentOpenCSGAccessToken

type remoteSkillsListResponse struct {
	Items    []remoteSkillSummary `json:"items"`
	NextPage *int                 `json:"next_page,omitempty"`
	Page     int                  `json:"page"`
	Per      int                  `json:"per"`
	Total    *int                 `json:"total,omitempty"`
}

type remoteSkillSummary struct {
	Recommended bool   `json:"recommended"`
	Version     string `json:"version,omitempty"`
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
	page, per, err := remoteHubPageOptions(r, remoteSkillsDefaultPage, remoteSkillsDefaultPer, remoteSkillsMaxPer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	baseURL, token, err := h.remoteSkillsHubConnection(r)
	if err != nil {
		writeRemoteSkillsHubError(w, err)
		return
	}
	list, err := skillremote.ListAgenticHubSkills(r.Context(), baseURL, token, skillremote.AgenticHubSkillListOptions{
		Page:   page,
		Per:    per,
		Search: r.URL.Query().Get("search"),
	})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, skillremote.ErrAgenticHubSignInRequired) {
			status = http.StatusUnauthorized
		}
		http.Error(w, err.Error(), status)
		return
	}
	// Hub's catalog may omit release metadata that is available on the detail API.
	var versions sync.WaitGroup
	jobs := make(chan int)
	for range min(4, len(list.Items)) {
		versions.Add(1)
		go func() {
			defer versions.Done()
			for index := range jobs {
				if list.Items[index].Version != "" {
					continue
				}
				if releases, err := skillremote.GetAgenticHubSkillReleases(r.Context(), baseURL, token, list.Items[index].RemotePath); err == nil {
					list.Items[index].Version = releases.LatestVersion
				}
			}
		}()
	}
	for index := range list.Items {
		jobs <- index
	}
	close(jobs)
	versions.Wait()
	siteBaseURL := h.currentOpenCSGEnvironment(r).OpenCSGBaseURL
	items := make([]remoteSkillSummary, 0, len(list.Items))
	for _, item := range list.Items {
		remoteURL, err := skillremote.AgenticHubSkillWebURL(siteBaseURL, item.RemotePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		items = append(items, remoteSkillSummary{
			Version:     item.Version,
			Description: item.Description,
			Recommended: item.Recommended,
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
	baseURL, token, err := h.remoteSkillsHubConnection(r)
	if err != nil {
		writeRemoteSkillsHubError(w, err)
		return
	}
	version, commit := "", ""
	releases, releaseErr := skillremote.GetAgenticHubSkillReleases(r.Context(), baseURL, token, remotePath)
	if releaseErr != nil && !strings.Contains(releaseErr.Error(), "status 404") {
		http.Error(w, "skill release information is unavailable", http.StatusBadGateway)
		return
	}
	if release, ok := releases.Latest(); ok && (req.Ref == "" || req.Ref == "main" || req.Ref == releases.DefaultBranch || req.Ref == release.Commit) {
		version, commit, ref = release.Version, release.Commit, release.Commit
	}
	for _, release := range releases.Versions {
		if release.Version == req.Ref || release.Commit == req.Ref {
			version, commit, ref = release.Version, release.Commit, release.Commit
			break
		}
	}
	archive, err := skillremote.FetchAgenticHubSkillArchive(r.Context(), baseURL, token, remotePath, ref)
	if err != nil {
		if skillremote.IsInvalidAgenticHubRequest(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	sourceURL, err := skillremote.AgenticHubSkillWebURL(h.currentOpenCSGEnvironment(r).OpenCSGBaseURL, remotePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	item, err := skilllocal.InstallArchiveWithOptions(
		root,
		skillremote.AgenticHubSkillArchiveName(remotePath),
		archive,
		skilllocal.InstallArchiveOptions{Replace: req.Replace, Origin: skillinstall.Record{
			Source:     skillinstall.SourceMarket,
			SourceURL:  sourceURL,
			Ref:        ref,
			RemotePath: remotePath,
			Version:    version,
			Commit:     commit,
		}},
	)
	if err != nil {
		writeSkillInstallError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) remoteSkillsHubConnection(r *http.Request) (string, string, error) {
	registry, err := h.remoteHubRegistryForRequest(r)
	if err != nil {
		return "", "", err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(registry.URL), "/")
	if baseURL == "" {
		return "", "", errOfficialHubURLNotConfigured
	}
	status, err := appAuthStatus(r)
	if err != nil {
		return "", "", err
	}
	if !status.Authenticated || !hubRegistryMatchesAuthenticatedEnvironment(registry, status) {
		return baseURL, "", nil
	}
	token, err := remoteSkillsHubAccessToken()
	if err != nil {
		return "", "", err
	}
	return baseURL, strings.TrimSpace(token), nil
}

func (h *Handler) handleRemoteSkillBrowse(w http.ResponseWriter, r *http.Request, file bool) {
	baseURL, token, err := h.remoteSkillsHubConnection(r)
	if err != nil {
		writeRemoteSkillsHubError(w, err)
		return
	}
	query := r.URL.Query()
	var value any
	if file {
		value, err = skillremote.ReadAgenticHubSkillFile(r.Context(), baseURL, token, query.Get("remote_path"), query.Get("ref"), query.Get("path"))
	} else {
		value, err = skillremote.ListAgenticHubSkillFiles(r.Context(), baseURL, token, query.Get("remote_path"), query.Get("ref"))
	}
	if err != nil {
		if skillremote.IsInvalidAgenticHubRequest(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func writeRemoteSkillsHubError(w http.ResponseWriter, err error) {
	if errors.Is(err, errOfficialHubURLNotConfigured) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
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

package api

import (
	"errors"
	"net/http"
	"strings"

	"csgclaw/internal/mcp"
)

const (
	remoteMCPServersDefaultPage = 1
	remoteMCPServersDefaultPer  = 12
	remoteMCPServersMaxPer      = 100
)

var (
	errRemoteMCPHubNotConfigured  = errors.New("remote MCP Hub URL is not configured")
	errRemoteMCPHubSignInRequired = errors.New("OpenCSG sign-in is required to browse remote MCP servers")
	remoteMCPHubAccessToken       = currentOpenCSGAccessToken
)

type remoteMCPServersListResponse struct {
	Items    []remoteMCPServerSummary `json:"items"`
	NextPage *int                     `json:"next_page,omitempty"`
	Page     int                      `json:"page"`
	Per      int                      `json:"per"`
	Total    *int                     `json:"total,omitempty"`
}

type remoteMCPServerSummary struct {
	Description string `json:"description,omitempty"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol,omitempty"`
	URL         string `json:"url,omitempty"`
}

// remoteMCPServerInstallResponse deliberately excludes the resolved server
// configuration so that the Hub detail response, which can include headers,
// is not echoed by the installation endpoint.
type remoteMCPServerInstallResponse struct {
	Name string `json:"name"`
}

func (h *Handler) handleRemoteMCPServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page, per, err := remoteHubPageOptions(r, remoteMCPServersDefaultPage, remoteMCPServersDefaultPer, remoteMCPServersMaxPer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	baseURL, token, err := h.remoteMCPHubConnection(r)
	if err != nil {
		writeRemoteMCPHubError(w, err)
		return
	}
	list, err := mcp.ListRemoteServers(r.Context(), baseURL, token, mcp.RemoteServerListOptions{
		Page:   page,
		Per:    per,
		Search: r.URL.Query().Get("search"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	items := make([]remoteMCPServerSummary, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, remoteMCPServerSummary{
			Description: item.Description,
			ID:          item.ID,
			Name:        item.Name,
			Protocol:    item.Protocol,
			URL:         item.URL,
		})
	}
	writeJSON(w, http.StatusOK, remoteMCPServersListResponse{
		Items:    items,
		NextPage: nextRemoteMCPServersPage(page, per, list.Total, list.RecordCount),
		Page:     page,
		Per:      per,
		Total:    list.Total,
	})
}

// handleInstallRemoteMCPServer resolves the selected Hub item on the server,
// then stores its full configuration in the local MCP catalog. The Hub detail
// is never requested directly by the browser.
func (h *Handler) handleInstallRemoteMCPServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.mcp == nil {
		http.Error(w, "mcp service is not configured", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimSpace(pathValue(r, "id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	baseURL, token, err := h.remoteMCPHubConnection(r)
	if err != nil {
		writeRemoteMCPHubError(w, err)
		return
	}
	server, err := mcp.GetRemoteServer(r.Context(), baseURL, token, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	name, err := h.mcp.InstallRemoteServer(r.Context(), server)
	if err != nil {
		writeMCPServerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, remoteMCPServerInstallResponse{
		Name: name,
	})
}

func writeRemoteMCPHubError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errRemoteMCPHubSignInRequired):
		http.Error(w, err.Error(), http.StatusUnauthorized)
	case errors.Is(err, errRemoteMCPHubNotConfigured):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func nextRemoteMCPServersPage(page, per int, total *int, recordCount int) *int {
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

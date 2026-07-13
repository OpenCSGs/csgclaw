package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"csgclaw/internal/mcp"
)

type mcpServerRequest struct {
	Config map[string]any `json:"config"`
	Name   string         `json:"name"`
}

func (h *Handler) handleMCPServers(w http.ResponseWriter, r *http.Request) {
	if h.mcp == nil {
		http.Error(w, "mcp service is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		servers, err := h.mcp.ListServers(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{mcp.ServersKey: servers})
	case http.MethodPost:
		req, err := decodeMCPServerRequest(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		state, err := h.mcp.CreateServer(r.Context(), req.Name, req.Config)
		if err != nil {
			writeMCPServerError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, state)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleMCPServerByName(w http.ResponseWriter, r *http.Request) {
	if h.mcp == nil {
		http.Error(w, "mcp service is not configured", http.StatusServiceUnavailable)
		return
	}
	name := strings.TrimSpace(pathValue(r, "name"))
	if name == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		req, err := decodeMCPServerRequest(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
			return
		}
		nextName := strings.TrimSpace(req.Name)
		if nextName == "" {
			nextName = name
		}
		state, err := h.mcp.UpdateServer(r.Context(), name, nextName, req.Config)
		if err != nil {
			writeMCPServerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	case http.MethodDelete:
		state, err := h.mcp.DeleteServer(r.Context(), name)
		if err != nil {
			writeMCPServerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func decodeMCPServerRequest(r *http.Request) (mcpServerRequest, error) {
	var req mcpServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Config == nil {
		return req, fmt.Errorf("config is required")
	}
	return req, nil
}

func writeMCPServerError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, mcp.ErrServerExists) {
		status = http.StatusConflict
	}
	if errors.Is(err, mcp.ErrServerNotFound) {
		status = http.StatusNotFound
	}
	http.Error(w, err.Error(), status)
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"csgclaw/internal/utils/filebrowse"
)

// Native applications run on the service host, never on a remote web client.
// Only a literal loopback host and loopback peer qualify, including in NoAuth mode.
func localWorkspaceClient(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(peer).IsLoopback() {
		return false
	}
	host := r.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	return host == "localhost" || net.ParseIP(strings.Trim(host, "[]")).IsLoopback()
}

// NoAuth is an access policy, not proof that an explicit credential is valid.
// Native open actions may skip Origin only for a configured management token.
func (h *Handler) validWorkspaceOpenCredential(header string) bool {
	for _, configured := range []string{h.serverAccessToken, h.desktopSessionToken} {
		if token := strings.TrimSpace(configured); token != "" && header == "Bearer "+token {
			return true
		}
	}
	return false
}

func (h *Handler) workspaceOpenTargets(r *http.Request, path string, directory bool) []workspaceOpenTarget {
	if !localWorkspaceClient(r) || appRequestAgentID(r) != "" || r.Header.Get("X-CSGClaw-Caller-Agent") != "" || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer agent.") {
		return []workspaceOpenTarget{}
	}
	if header := r.Header.Get("Authorization"); header != "" && !h.validWorkspaceOpenCredential(header) {
		return []workspaceOpenTarget{}
	}
	targets := []workspaceOpenTarget{}
	switch runtime.GOOS {
	case "darwin":
		targets = append(targets, workspaceOpenTarget{ID: "system"}, workspaceOpenTarget{ID: "terminal"})
	case "windows":
		targets = append(targets, workspaceOpenTarget{ID: "system"})
		if _, err := exec.LookPath("wt.exe"); err == nil {
			targets = append(targets, workspaceOpenTarget{ID: "terminal"})
		}
	case "linux":
		if linuxDisplayAvailable(os.Getenv, connectDisplay) {
			if _, err := exec.LookPath("xdg-open"); err == nil {
				targets = append(targets, workspaceOpenTarget{ID: "system"})
			}
		}
	}
	if !directory && len(targets) > 0 {
		targets = append(targets, associatedWorkspaceApplications(r.Context(), path)...)
	}
	return targets
}

type workspaceLocation struct {
	AbsolutePath string                `json:"absolute_path"`
	Directory    bool                  `json:"directory"`
	OpenTargets  []workspaceOpenTarget `json:"open_targets"`
}

func (h *Handler) resolveWorkspaceLocation(r *http.Request, relative string) (workspaceLocation, error) {
	if h.svc == nil || h.workspace == nil {
		return workspaceLocation{}, errors.New("workspace unavailable")
	}
	root, err := h.conversationWorkspaceRoot(strings.TrimSpace(pathValue(r, "id")), r.URL.Query().Get("room_id"))
	if err != nil {
		return workspaceLocation{}, err
	}
	file, err := filebrowse.OpenEntry(root, relative)
	if err != nil {
		return workspaceLocation{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return workspaceLocation{}, err
	}
	absolute, err := filepath.Abs(file.Name())
	if err != nil {
		return workspaceLocation{}, err
	}
	return workspaceLocation{absolute, info.IsDir(), h.workspaceOpenTargets(r, absolute, info.IsDir())}, nil
}

func (h *Handler) handleAgentWorkspaceLocation(w http.ResponseWriter, r *http.Request) {
	location, err := h.resolveWorkspaceLocation(r, r.URL.Query().Get("path"))
	if err != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "workspace_file_unavailable", "Workspace path is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, location)
}

func (h *Handler) handleAgentWorkspaceOpen(w http.ResponseWriter, r *http.Request) {
	h.handleWorkspaceOpen(w, r, func(path string) (workspaceLocation, error) { return h.resolveWorkspaceLocation(r, path) })
}

// Application launches require a local peer, an allowed host, and either a
// management credential or an exact same-origin browser request.
func (h *Handler) workspaceOpenRequestAllowed(r *http.Request) bool {
	if len(h.workspaceOpenTargets(r, "", true)) == 0 {
		return false
	}
	if h.validWorkspaceOpenCredential(r.Header.Get("Authorization")) {
		return true
	}
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && (origin.Scheme == "http" || origin.Scheme == "https") && origin.Host == r.Host
}

func (h *Handler) handleWorkspaceOpen(w http.ResponseWriter, r *http.Request, resolve func(string) (workspaceLocation, error)) {
	// Browser requests require an exact same-origin JSON POST. Origin is a CSRF
	// boundary here, in addition to the loopback-only execution boundary; it never
	// grants attachment reads or overrides an explicit invalid/scoped credential.
	if !h.workspaceOpenRequestAllowed(r) {
		writeCodedAPIError(w, http.StatusForbidden, "workspace_open_denied", "Local workspace opening is unavailable")
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeCodedAPIError(w, http.StatusBadRequest, "invalid_request", "JSON is required")
		return
	}
	var input struct {
		Path   string `json:"path"`
		Target string `json:"target"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&input) != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "invalid_request", "Invalid workspace open request")
		return
	}
	location, err := resolve(input.Path)
	if err != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "workspace_file_unavailable", "Workspace path is unavailable")
		return
	}
	allowed := false
	for _, target := range location.OpenTargets {
		if target.ID == input.Target {
			allowed = true
		}
	}
	if !allowed {
		writeCodedAPIError(w, http.StatusForbidden, "workspace_open_denied", "Application is unavailable")
		return
	}
	opener := h.workspaceOpener
	if opener == nil {
		opener = openWorkspacePath
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err = opener(ctx, location.AbsolutePath, input.Target, location.Directory); err != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "workspace_open_failed", "Could not open workspace path")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Build argument vectors, never shell command strings, including for terminal paths.
func workspaceOpenCommand(goos, path, target string, directory bool) (string, []string, error) {
	parent := path
	if !directory {
		parent = filepath.Dir(path)
	}
	switch goos {
	case "darwin":
		switch target {
		case "system":
			if directory {
				return "open", []string{"-a", "Finder", path}, nil
			}
			return "open", []string{"-R", path}, nil
		case "terminal":
			return "open", []string{"-a", "Terminal", parent}, nil
		}
		if strings.HasPrefix(target, "app:") && filepath.IsAbs(strings.TrimPrefix(target, "app:")) {
			return "open", []string{"-a", strings.TrimPrefix(target, "app:"), path}, nil
		}
	case "windows":
		switch target {
		case "system":
			if directory {
				return "explorer.exe", []string{path}, nil
			}
			return "explorer.exe", []string{"/select," + path}, nil
		case "default":
			return "rundll32.exe", []string{"url.dll,FileProtocolHandler", path}, nil
		case "terminal":
			// wt.exe parses command separators inside each argv element and decodes
			// \; back to a literal semicolon after splitting its command list.
			return "wt.exe", []string{"-d", strings.ReplaceAll(parent, ";", `\;`)}, nil
		}
	case "linux":
		if target == "default" {
			return "xdg-open", []string{path}, nil
		}
		if target == "system" {
			return "xdg-open", []string{parent}, nil
		}
	}
	return "", nil, errors.New("unsupported workspace application")
}

func openWorkspacePath(ctx context.Context, path, target string, directory bool) error {
	command, args, err := workspaceOpenCommand(runtime.GOOS, path, target, directory)
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return exec.CommandContext(ctx, command, args...).Run()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	child := exec.Command(command, args...)
	if err := child.Start(); err != nil {
		return err
	}
	go func() { _ = child.Wait() }()
	return nil
}

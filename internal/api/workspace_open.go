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

func (h *Handler) workspaceOpenTargets(r *http.Request) []string {
	if !localWorkspaceClient(r) || appRequestAgentID(r) != "" || r.Header.Get("X-CSGClaw-Caller-Agent") != "" || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer agent.") {
		return []string{}
	}
	if header := r.Header.Get("Authorization"); header != "" && !h.validWorkspaceOpenCredential(header) {
		return []string{}
	}
	targets := []string{}
	switch runtime.GOOS {
	case "darwin":
		targets = append(targets, "system", "terminal")
		for _, app := range []struct{ id, name string }{{"vscode", "Visual Studio Code"}, {"sublime", "Sublime Text"}, {"iterm", "iTerm"}} {
			for _, dir := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
				if _, err := os.Stat(filepath.Join(dir, app.name+".app")); err == nil {
					targets = append(targets, app.id)
					break
				}
			}
		}
	case "windows":
		targets = append(targets, "system", "notepad")
		if _, err := exec.LookPath("wt.exe"); err == nil {
			targets = append(targets, "terminal")
		}
	case "linux":
		if linuxDisplayAvailable(os.Getenv, connectDisplay) {
			if _, err := exec.LookPath("xdg-open"); err == nil {
				targets = append(targets, "system")
			}
		}
	}
	if runtime.GOOS != "darwin" {
		if _, err := exec.LookPath("code"); err == nil && runtime.GOOS != "windows" {
			targets = append(targets, "vscode")
		}
	}
	return targets
}

type workspaceLocation struct {
	AbsolutePath string   `json:"absolute_path"`
	Directory    bool     `json:"directory"`
	OpenTargets  []string `json:"open_targets"`
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
	return workspaceLocation{absolute, info.IsDir(), h.workspaceOpenTargets(r)}, nil
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
	// Browser requests require an exact same-origin JSON POST. Origin is a CSRF
	// boundary here, in addition to the loopback-only execution boundary; it never
	// grants attachment reads or overrides an explicit invalid/scoped credential.
	origin, err := url.Parse(r.Header.Get("Origin"))
	authenticated := h.validWorkspaceOpenCredential(r.Header.Get("Authorization"))
	if !localWorkspaceClient(r) || (!authenticated && (err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Host != r.Host)) {
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
	location, err := h.resolveWorkspaceLocation(r, input.Path)
	if err != nil {
		writeCodedAPIError(w, http.StatusBadRequest, "workspace_file_unavailable", "Workspace path is unavailable")
		return
	}
	allowed := false
	for _, target := range location.OpenTargets {
		if target == input.Target {
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
		case "vscode":
			return "open", []string{"-a", "Visual Studio Code", path}, nil
		case "sublime":
			return "open", []string{"-a", "Sublime Text", path}, nil
		case "terminal":
			return "open", []string{"-a", "Terminal", parent}, nil
		case "iterm":
			return "open", []string{"-a", "iTerm", parent}, nil
		}
	case "windows":
		switch target {
		case "system":
			if directory {
				return "explorer.exe", []string{path}, nil
			}
			return "explorer.exe", []string{"/select," + path}, nil
		case "notepad":
			if !directory {
				return "notepad.exe", []string{path}, nil
			}
		case "vscode":
			return "code", []string{path}, nil
		case "terminal":
			// wt.exe parses command separators inside each argv element and decodes
			// \; back to a literal semicolon after splitting its command list.
			return "wt.exe", []string{"-d", strings.ReplaceAll(parent, ";", `\;`)}, nil
		}
	case "linux":
		if target == "system" {
			return "xdg-open", []string{parent}, nil
		}
		if target == "vscode" {
			return "code", []string{path}, nil
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

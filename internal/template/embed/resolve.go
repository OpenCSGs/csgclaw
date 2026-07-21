package templateembed

import (
	"fmt"
	"io/fs"
	pathpkg "path"
	"slices"
	"strings"

	runtimepkg "csgclaw/internal/runtime"
)

const (
	roleWorker  = "worker"
	roleManager = "manager"
)

type BuiltinTemplate struct {
	ID          string
	RuntimeKind string
	Role        string
	Root        string
}

var builtinTemplates = []BuiltinTemplate{
	{
		ID:          "manager-codex",
		RuntimeKind: runtimepkg.KindCodex,
		Role:        roleManager,
		Root:        CodexManagerRoot,
	},
	{
		ID:          "codex-worker",
		RuntimeKind: runtimepkg.KindCodex,
		Role:        roleWorker,
		Root:        CodexWorkerRoot,
	},
	{
		ID:          "openclaw-worker",
		RuntimeKind: runtimepkg.KindOpenClawSandbox,
		Role:        roleWorker,
		Root:        OpenClawWorkerRoot,
	},
}

func FS() fs.FS {
	return runtimeTemplateFS
}

func Builtins() []BuiltinTemplate {
	return slices.Clone(builtinTemplates)
}

func LookupBuiltin(id string) (BuiltinTemplate, bool) {
	id = strings.TrimSpace(id)
	for _, item := range builtinTemplates {
		if item.ID == id {
			return item, true
		}
	}
	return BuiltinTemplate{}, false
}

func Resolve(runtimeKind, role string) (string, error) {
	runtimeKind = strings.TrimSpace(runtimeKind)
	role = normalizeRole(role)
	for _, item := range builtinTemplates {
		if item.RuntimeKind == runtimeKind && item.Role == role {
			return item.Root, nil
		}
	}
	return "", fmt.Errorf("runtime template not found for runtime kind %q role %q", runtimeKind, role)
}

func ManifestPath(templateRoot string) string {
	return strings.TrimRight(strings.TrimSpace(templateRoot), "/") + "/" + ManifestFileName
}

func WorkspacePath(templateRoot string) string {
	return strings.TrimRight(strings.TrimSpace(templateRoot), "/")
}

func TemplateIDFromManifestPath(path string) string {
	path = pathpkg.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return ""
	}
	return pathpkg.Base(pathpkg.Dir(path))
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "agent":
		return "agent"
	case roleWorker:
		return roleWorker
	case roleManager:
		return roleManager
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

package templates

import (
	"fmt"
	"io/fs"
	pathpkg "path"
	"strings"

	runtimepkg "csgclaw/internal/runtime"
)

const (
	roleWorker  = "worker"
	roleManager = "manager"
)

func FS() fs.FS {
	return runtimeTemplateFS
}

func Resolve(runtimeKind, role string) (string, error) {
	switch runtimeKind {
	case runtimepkg.KindPicoClawSandbox:
		switch normalizeRole(role) {
		case roleManager:
			return PicoClawManagerRoot, nil
		case roleWorker:
			return PicoClawWorkerRoot, nil
		}
	case runtimepkg.KindOpenClawSandbox:
		switch normalizeRole(role) {
		case roleWorker:
			return OpenClawWorkerRoot, nil
		case roleManager:
			return OpenClawManagerRoot, nil
		}
	}
	return "", fmt.Errorf("runtime template not found for runtime kind %q role %q", runtimeKind, role)
}

func ManifestPath(templateRoot string) string {
	return strings.TrimRight(strings.TrimSpace(templateRoot), "/") + "/" + ManifestFileName
}

func WorkspacePath(templateRoot string) string {
	return strings.TrimRight(strings.TrimSpace(templateRoot), "/") + "/" + WorkspaceDirName
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

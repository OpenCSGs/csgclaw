package dockercli

import (
	"fmt"
	"sort"
	"strings"

	"csgclaw/internal/sandbox"
)

func runArgs(spec sandbox.CreateSpec) ([]string, error) {
	if strings.TrimSpace(spec.Image) == "" {
		return nil, fmt.Errorf("invalid sandbox image: image is required")
	}
	if len(spec.Entrypoint) > 0 {
		return nil, fmt.Errorf("unsupported sandbox option: entrypoint")
	}

	args := []string{"run"}
	if strings.TrimSpace(spec.Name) != "" {
		args = append(args, "--name", spec.Name)
	}
	if spec.Detach {
		args = append(args, "--detach")
	}
	if spec.AutoRemove {
		args = append(args, "--rm")
	}

	keys := make([]string, 0, len(spec.Env))
	for key := range spec.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid sandbox env: key is required")
		}
		args = append(args, "-e", key+"="+spec.Env[key])
	}

	for _, mount := range spec.Mounts {
		if strings.TrimSpace(mount.HostPath) == "" {
			return nil, fmt.Errorf("invalid sandbox mount: host path is required")
		}
		if strings.TrimSpace(mount.GuestPath) == "" {
			return nil, fmt.Errorf("invalid sandbox mount: guest path is required")
		}
		value := mount.HostPath + ":" + mount.GuestPath
		if mount.ReadOnly {
			value += ":ro"
		}
		args = append(args, "-v", value)
	}

	args = append(args, spec.Image)
	args = append(args, spec.Cmd...)
	return args, nil
}

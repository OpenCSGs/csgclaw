package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func platformFileApplications(ctx context.Context, path string) []workspaceOpenTarget {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	mime, err := exec.CommandContext(ctx, "xdg-mime", "query", "filetype", path).Output()
	if err != nil {
		return nil
	}
	desktop, err := exec.CommandContext(ctx, "xdg-mime", "query", "default", strings.TrimSpace(string(mime))).Output()
	if err != nil {
		return nil
	}
	id := strings.TrimSpace(string(desktop))
	if id == "" || filepath.Base(id) != id {
		return nil
	}
	home, _ := os.UserHomeDir()
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	for _, dir := range append([]string{dataHome}, filepath.SplitList(dataDirs)...) {
		data, err := os.ReadFile(filepath.Join(dir, "applications", id))
		if err != nil {
			continue
		}
		inEntry := false
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "[") {
				inEntry = strings.TrimSpace(line) == "[Desktop Entry]"
			}
			if inEntry && strings.HasPrefix(line, "Name=") {
				return []workspaceOpenTarget{{ID: "default", Label: strings.TrimSpace(strings.TrimPrefix(line, "Name=")), Default: true}}
			}
		}
	}
	return nil
}

package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	bundleMarkerFileName = ".csgclaw-bundle.json"
	bundleMarkerApp      = "csgclaw"
	bundleMarkerLayout   = "official-bundle"
)

type bundleMarker struct {
	App     string `json:"app"`
	Layout  string `json:"layout"`
	Version string `json:"version,omitempty"`
}

func bundleMarkerPath(bundleDir string) string {
	return filepath.Join(bundleDir, bundleMarkerFileName)
}

func validateBundleMarker(bundleDir string) error {
	data, err := os.ReadFile(bundleMarkerPath(bundleDir))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("release bundle is missing %s", bundleMarkerFileName)
		}
		return fmt.Errorf("read bundle marker: %w", err)
	}
	var marker bundleMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return fmt.Errorf("decode bundle marker: %w", err)
	}
	if strings.TrimSpace(marker.App) != bundleMarkerApp {
		return fmt.Errorf("invalid bundle marker app %q", marker.App)
	}
	if strings.TrimSpace(marker.Layout) != bundleMarkerLayout {
		return fmt.Errorf("invalid bundle marker layout %q", marker.Layout)
	}
	return nil
}

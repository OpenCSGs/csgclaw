package skill

import (
	"strings"
	"time"

	"csgclaw/internal/skill/installstate"
)

func newInstallRecord(sourceURL, slug, version, sha256 string) InstallRecord {
	return InstallRecord{
		Source:      installstate.SourceMarket,
		SourceURL:   sourceURL,
		Slug:        normalizeSlug(slug),
		Version:     strings.TrimSpace(version),
		InstalledAt: time.Now().UTC(),
		SHA256:      sha256,
	}
}

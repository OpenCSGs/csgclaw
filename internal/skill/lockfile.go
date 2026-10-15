package skill

import (
	"strings"
	"time"

	"csgclaw/internal/skill/installstate"
)

func writeLockRecord(skillsRoot string, record InstallRecord) error {
	return installstate.Update(skillsRoot, record.Slug, &record, nil)
}

func newInstallRecord(registry RegistryID, slug, version, sha256 string) InstallRecord {
	return InstallRecord{
		Source:      installstate.SourceMarket,
		Registry:    string(registry),
		Slug:        normalizeSlug(slug),
		Version:     strings.TrimSpace(version),
		InstalledAt: time.Now().UTC(),
		SHA256:      sha256,
	}
}

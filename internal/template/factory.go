package template

import (
	"csgclaw/internal/migrations/retiredclaws"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"csgclaw/internal/config"
)

var (
	ErrRegistryKindUnsupported = errors.New("hub registry kind is not supported yet")
	ErrRegistryPathRequired    = errors.New("hub registry path is required")
	ErrRegistryURLRequired     = errors.New("hub registry url is required")
)

func DefaultStoreFactory(cfg config.HubRegistryConfig) (Store, error) {
	switch normalizeRegistryKind(cfg.Kind) {
	case RegistryKindBuiltin:
		return NewBuiltinStore(), nil
	case RegistryKindLocal:
		path := strings.TrimSpace(cfg.Path)
		if path == "" {
			return nil, ErrRegistryPathRequired
		}
		if err := retiredclaws.LocalTemplates(path); err != nil {
			if !errors.Is(err, os.ErrPermission) {
				return nil, err
			}
			slog.Warn("local template registry is read-only; adapting retired manifests in memory", "path", path)
		}
		return NewLocalStore(path), nil
	case RegistryKindRemote:
		baseURL := strings.TrimSpace(cfg.URL)
		if baseURL == "" {
			return nil, ErrRegistryURLRequired
		}
		return NewRemoteStore(baseURL, cfg.Token), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrRegistryKindUnsupported, cfg.Kind)
	}
}

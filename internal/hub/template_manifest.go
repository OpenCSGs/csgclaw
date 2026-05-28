package hub

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/runtime"
)

type templateManifest struct {
	Name        string               `toml:"name"`
	Description string               `toml:"description,omitempty"`
	Role        string               `toml:"role"`
	RuntimeKind string               `toml:"runtime_kind"`
	Image       templateImageSection `toml:"image"`
	UpdatedAt   string               `toml:"updated_at,omitempty"`
}

type templateImageSection struct {
	Ref       string                 `toml:"ref"`
	Digest    string                 `toml:"digest,omitempty"`
	Platforms []string               `toml:"platforms,omitempty"`
	Env       []templateImageEnvItem `toml:"env"`
}

type templateImageEnvItem struct {
	Name        string   `toml:"name"`
	Required    bool     `toml:"required"`
	Secret      bool     `toml:"secret"`
	Default     string   `toml:"default,omitempty"`
	Description string   `toml:"description,omitempty"`
	Choices     []string `toml:"choices,omitempty"`
	Pattern     string   `toml:"pattern,omitempty"`
	Example     string   `toml:"example,omitempty"`
	Placeholder string   `toml:"placeholder,omitempty"`
}

func manifestImageRef(image templateImageSection) string {
	return strings.TrimSpace(image.Ref)
}

func manifestImageEnv(image templateImageSection) []apitypes.ImageEnvContract {
	items := normalizeImageEnvContracts(image.Env)
	if len(items) == 0 {
		return nil
	}
	return items
}

func normalizeImageEnvContracts(raw []templateImageEnvItem) []apitypes.ImageEnvContract {
	if len(raw) == 0 {
		return nil
	}
	out := make([]apitypes.ImageEnvContract, 0, len(raw))
	for _, item := range raw {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		contract := apitypes.ImageEnvContract{
			Name:        name,
			Required:    item.Required,
			Secret:      item.Secret,
			Default:     strings.TrimSpace(item.Default),
			Description: strings.TrimSpace(item.Description),
			Pattern:     strings.TrimSpace(item.Pattern),
			Example:     strings.TrimSpace(item.Example),
			Placeholder: strings.TrimSpace(item.Placeholder),
		}
		if len(item.Choices) > 0 {
			contract.Choices = append([]string(nil), item.Choices...)
		}
		out = append(out, contract)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func validateImageEnvContracts(items []templateImageEnvItem) error {
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return fmt.Errorf("image.env[%d].name is required", index)
		}
		normalized := strings.ToUpper(name)
		if _, ok := seen[normalized]; ok {
			return fmt.Errorf("duplicate image.env name %q", name)
		}
		seen[normalized] = struct{}{}

		if item.Secret && strings.TrimSpace(item.Default) != "" {
			return fmt.Errorf("image.env[%d] secret entries cannot set default", index)
		}
		if len(item.Choices) == 0 {
			continue
		}
		if strings.TrimSpace(item.Default) != "" {
			defaultValue := strings.TrimSpace(item.Default)
			found := false
			for _, choice := range item.Choices {
				if choice == defaultValue {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("image.env[%d].default must appear in choices", index)
			}
		}
		pattern := strings.TrimSpace(item.Pattern)
		if pattern == "" {
			continue
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("image.env[%d].pattern is invalid: %w", index, err)
		}
	}
	return nil
}

func validateManifest(manifest templateManifest) error {
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.Name == "" {
		return ErrTemplateNameRequired
	}
	switch normalizeTemplateRole(manifest.Role) {
	case TemplateRoleManager, TemplateRoleWorker:
	default:
		return fmt.Errorf("role must be one of %q or %q", TemplateRoleManager, TemplateRoleWorker)
	}
	switch manifest.RuntimeKind {
	case runtime.KindPicoClawSandbox, runtime.KindOpenClawSandbox, runtime.KindCodex:
	default:
		return fmt.Errorf("%w: %s", ErrRuntimeKindRequired, manifest.RuntimeKind)
	}
	imageRef := manifestImageRef(manifest.Image)
	if requiresTemplateImage(manifest.RuntimeKind) && imageRef == "" {
		return fmt.Errorf("image.ref is required for runtime_kind %q", manifest.RuntimeKind)
	}
	if err := validateImageEnvContracts(manifest.Image.Env); err != nil {
		return err
	}
	if _, err := parseManifestUpdatedAt(manifest.UpdatedAt); err != nil {
		return err
	}
	return nil
}

func requiresTemplateImage(runtimeKind string) bool {
	switch runtimeKind {
	case runtime.KindPicoClawSandbox, runtime.KindOpenClawSandbox:
		return true
	default:
		return false
	}
}

func parseManifestUpdatedAt(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid updated_at %q", value)
	}
	return parsed.UTC(), nil
}

const (
	TemplateRoleManager = "manager"
	TemplateRoleWorker  = "worker"
)

func normalizeTemplateRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case TemplateRoleManager:
		return TemplateRoleManager
	case TemplateRoleWorker:
		return TemplateRoleWorker
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

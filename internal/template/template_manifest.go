package template

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/runtime"
)

const currentAgentFileSchemaVersion = "agentfile/v1"

var publishTemplateNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,23}$`)

func ValidatePublishTemplateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrTemplateNameRequired
	}
	if !publishTemplateNamePattern.MatchString(name) {
		return ErrTemplateNameInvalid
	}
	return nil
}

type templateManifest struct {
	SchemaVersion  string               `toml:"schema_version,omitempty"`
	Name           string               `toml:"name"`
	Description    string               `toml:"description,omitempty"`
	RuntimeKind    string               `toml:"runtime_kind"`
	Version        string               `toml:"version,omitempty"`
	Image          templateImageSection `toml:"image"`
	RuntimeOptions map[string]any       `toml:"runtime_options,omitempty"`
	UpdatedAt      string               `toml:"updated_at,omitempty"`
}

type templateImageSection struct {
	Ref string                 `toml:"ref"`
	Env []templateImageEnvItem `toml:"env"`
}

type templateImageEnvItem struct {
	Name     string `toml:"name"`
	Required bool   `toml:"required"`
	Secret   bool   `toml:"secret"`
	Default  string `toml:"default,omitempty"`
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
			Name:     name,
			Required: item.Required,
			Secret:   item.Secret,
			Default:  strings.TrimSpace(item.Default),
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
	}
	return nil
}

func validateManifest(manifest templateManifest) error {
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.Name == "" {
		return ErrTemplateNameRequired
	}
	manifest.RuntimeKind = normalizeTemplateRuntimeKind(manifest.RuntimeKind)
	switch manifest.RuntimeKind {
	case runtime.NamePicoClaw, runtime.NameOpenClaw, runtime.KindCodex:
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
	if _, err := normalizeTemplateRuntimeOptions(manifest.RuntimeKind, manifest.RuntimeOptions); err != nil {
		return err
	}
	if _, err := parseManifestUpdatedAt(manifest.UpdatedAt); err != nil {
		return err
	}
	return nil
}

func normalizeTemplateRuntimeOptions(runtimeKind string, raw map[string]any) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	runtimeKind = normalizeTemplateRuntimeKind(runtimeKind)
	if runtimeKind != runtime.KindCodex {
		return nil, fmt.Errorf("runtime_options are supported only for Codex worker templates")
	}
	if len(raw) != 1 {
		return nil, fmt.Errorf("runtime_options supports only execution_mode")
	}
	value, ok := raw["execution_mode"]
	if !ok {
		return nil, fmt.Errorf("runtime_options supports only execution_mode")
	}
	mode, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("runtime_options.execution_mode must be a string")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "standard", "read_only":
		return map[string]any{"execution_mode": mode}, nil
	default:
		return nil, fmt.Errorf("runtime_options.execution_mode must be %q or %q", "standard", "read_only")
	}
}

func cloneTemplateRuntimeOptions(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]any, len(raw))
	for key, value := range raw {
		out[key] = value
	}
	return out
}

func requiresTemplateImage(runtimeKind string) bool {
	return runtime.RuntimeConfigForKind(runtimeKind).Sandboxed
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

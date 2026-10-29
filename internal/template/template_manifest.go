package template

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
	"csgclaw/internal/runtime"
	runtimedsh "csgclaw/internal/runtime/dsh"
)

const currentAgentFileSchemaVersion = "agentfile/v1"

var publishTemplateNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)

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
	UID            string                        `toml:"template_uid,omitempty"`
	Revision       int                           `toml:"revision,omitempty"`
	SchemaVersion  string                        `toml:"schema_version,omitempty"`
	Name           string                        `toml:"name"`
	Avatar         string                        `toml:"avatar,omitempty"`
	DisplayName    string                        `toml:"display_name,omitempty"`
	Description    string                        `toml:"description,omitempty"`
	RuntimeKind    string                        `toml:"runtime_kind"`
	Version        string                        `toml:"version,omitempty"`
	Image          templateImageSection          `toml:"image"`
	RuntimeOptions map[string]any                `toml:"runtime_options,omitempty"`
	ModelConfig    *apitypes.TemplateModelConfig `toml:"model_config,omitempty"`
	UpdatedAt      string                        `toml:"updated_at,omitempty"`
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
	if !isSupportedTemplateRuntimeKind(manifest.RuntimeKind) {
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
	if _, err := normalizeTemplateModelConfig(manifest.ModelConfig); err != nil {
		return err
	}
	if _, err := parseManifestUpdatedAt(manifest.UpdatedAt); err != nil {
		return err
	}
	return nil
}

func isSupportedTemplateRuntimeKind(kind string) bool {
	switch normalizeTemplateRuntimeKind(kind) {
	case runtime.KindCodex, runtime.KindDSH:
		return true
	default:
		return false
	}
}

func CanPublishCommunityTemplateRuntime(kind string) bool {
	switch normalizeTemplateRuntimeKind(kind) {
	case runtime.KindCodex, runtime.KindDSH:
		return true
	default:
		return false
	}
}

func normalizeTemplateRuntimeOptions(runtimeKind string, raw map[string]any) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	runtimeKind = normalizeTemplateRuntimeKind(runtimeKind)
	if runtimeKind == runtime.KindDSH {
		for key := range raw {
			if key != runtimedsh.PermissionModeOptionKey && key != runtimedsh.MemoryModeOptionKey {
				return nil, fmt.Errorf("DSH runtime_options supports only permission_mode and memory_mode")
			}
		}
		opts, err := runtimedsh.DecodeRuntimeOptions(raw)
		if err != nil {
			return nil, fmt.Errorf("DSH runtime_options are invalid: %w", err)
		}
		return map[string]any{runtimedsh.PermissionModeOptionKey: opts.PermissionMode, runtimedsh.MemoryModeOptionKey: opts.MemoryMode}, nil
	}
	if runtimeKind != runtime.KindCodex {
		return nil, fmt.Errorf("runtime_options are supported only for Codex and DSH worker templates")
	}
	for key := range raw {
		if key != "execution_mode" && key != "memory_mode" {
			return nil, fmt.Errorf("runtime_options supports only execution_mode and memory_mode")
		}
	}
	out := make(map[string]any, len(raw))
	if value, ok := raw["execution_mode"]; ok {
		mode, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("runtime_options.execution_mode must be a string")
		}
		mode = strings.ToLower(strings.TrimSpace(mode))
		switch mode {
		case "standard", "read_only":
			out["execution_mode"] = mode
		default:
			return nil, fmt.Errorf("runtime_options.execution_mode must be %q or %q", "standard", "read_only")
		}
	}
	if value, ok := raw["memory_mode"]; ok {
		mode, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("runtime_options.memory_mode must be a string")
		}
		mode = strings.ToLower(strings.TrimSpace(mode))
		switch mode {
		case "enabled", "disabled":
			out["memory_mode"] = mode
		default:
			return nil, fmt.Errorf("runtime_options.memory_mode must be %q or %q", "enabled", "disabled")
		}
	}
	return out, nil
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

func normalizeTemplateModelConfig(raw *apitypes.TemplateModelConfig) (*apitypes.TemplateModelConfig, error) {
	if raw == nil {
		return nil, nil
	}
	normalized := &apitypes.TemplateModelConfig{
		ModelID:         strings.TrimSpace(raw.ModelID),
		ReasoningEffort: config.NormalizeReasoningEffort(raw.ReasoningEffort),
	}
	if normalized.ModelID == "" && normalized.ReasoningEffort == "" {
		return nil, nil
	}
	if !config.UsesModelReasoningDefault(normalized.ReasoningEffort) &&
		normalized.ReasoningEffort != config.ReasoningEffortNone &&
		!config.HasExplicitReasoningEffort(normalized.ReasoningEffort) {
		return nil, fmt.Errorf("model_config.reasoning_effort is invalid: %q", raw.ReasoningEffort)
	}
	return normalized, nil
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

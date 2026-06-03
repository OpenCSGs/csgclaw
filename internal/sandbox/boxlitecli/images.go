package boxlitecli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ListImages returns tagged local BoxLite image references in boxlite images order.
func ListImages(ctx context.Context, homeDir string, opts ...ProviderOption) ([]string, error) {
	p := NewProvider(opts...)
	return p.ListImages(ctx, homeDir)
}

// ListImages returns tagged local BoxLite image references in boxlite images order.
func (p Provider) ListImages(ctx context.Context, homeDir string) ([]string, error) {
	rt, err := p.Open(ctx, homeDir)
	if err != nil {
		return nil, err
	}
	result, err := rt.(*Runtime).run(ctx, []string{"images", "--format", "json"}, nil, nil)
	if err != nil {
		return nil, wrapRunError("list boxlite cli images", result, err)
	}
	images, err := parseImageRefs(result.Stdout)
	if err != nil {
		return nil, err
	}
	return images, nil
}

func parseImageRefs(data []byte) ([]string, error) {
	var entries []any
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse boxlite images json: %w", err)
	}

	images := []string{}
	seen := map[string]struct{}{}
	for _, entry := range entries {
		ref := imageRefFromEntry(entry)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		images = append(images, ref)
	}
	return images, nil
}

func imageRefFromEntry(entry any) string {
	switch value := entry.(type) {
	case string:
		return cleanImageRef(value)
	case map[string]any:
		for _, key := range []string{"Reference", "reference", "ImageRef", "image_ref", "Image", "image", "Name", "name"} {
			if ref := cleanImageRef(stringValue(value[key])); ref != "" {
				return ref
			}
		}
		repository := cleanImageRef(firstStringValue(value, "Repository", "repository"))
		if repository == "" {
			return ""
		}
		tag := cleanImageRef(firstStringValue(value, "Tag", "tag"))
		if tag == "" {
			return cleanImageRef(repository)
		}
		return cleanImageRef(repository + ":" + tag)
	default:
		return ""
	}
}

func firstStringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(values[key]); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func cleanImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "<none>") {
		return ""
	}
	return ref
}

package dockercli

import (
	"bufio"
	"context"
	"strings"
)

// ListImages returns tagged local Docker image references in docker image ls order.
func ListImages(ctx context.Context, opts ...ProviderOption) ([]string, error) {
	p := NewProvider(opts...)
	return p.ListImages(ctx, "")
}

// ListImages returns tagged local Docker image references in docker image ls order.
func (p Provider) ListImages(ctx context.Context, _ string) ([]string, error) {
	result, err := p.runner.Run(ctx, CommandRequest{
		Path: p.path,
		Args: []string{"image", "ls", "--format", "{{.Repository}}:{{.Tag}}"},
	})
	if err != nil {
		return nil, wrapRunError("docker image ls", result, err)
	}
	return parseImageRefs(result.Stdout), nil
}

func parseImageRefs(data []byte) []string {
	images := []string{}
	seen := map[string]struct{}{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		ref := strings.TrimSpace(scanner.Text())
		if ref == "" || strings.Contains(ref, "<none>") {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		images = append(images, ref)
	}
	return images
}

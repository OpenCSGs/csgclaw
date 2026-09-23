package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"csgclaw/internal/mcpschema"
)

func TestAvailabilityConfigHashSeparatesPresentationFromConnection(t *testing.T) {
	config := RemoteServer{ID: "42", Name: "必应搜索", HubURL: "https://hub.example", URL: "https://mcp.example"}.Config()
	config[mcpschema.DisplayNameKey] = "必应搜索"
	config["description"] = "Original description"
	original, _ := json.Marshal(config)
	want, err := availabilityConfigHash(config)
	if err != nil {
		t.Fatal(err)
	}
	if after, _ := json.Marshal(config); string(after) != string(original) {
		t.Fatal("hashing mutated stored metadata")
	}
	for _, key := range []string{mcpschema.DisplayNameKey, "description", ManagedMetaKey} {
		t.Run(key, func(t *testing.T) {
			changed := cloneMap(config)
			if key == ManagedMetaKey {
				changed[key] = map[string]any{ManagedMetaNamespace: map[string]any{"name": "new source name"}, mcpschema.MarketplaceMetaKey: map[string]any{"hub_url": "https://another-hub.example"}}
			} else {
				changed[key] = "Renamed 搜索"
			}
			got, err := availabilityConfigHash(changed)
			if err != nil || got != want {
				t.Fatalf("presentation edit invalidated availability: %v", err)
			}
			delete(changed, key)
			got, err = availabilityConfigHash(changed)
			if err != nil || got != want {
				t.Fatalf("removing presentation invalidated availability: %v", err)
			}
		})
	}
	for key, value := range map[string]any{
		"url": "https://other.example", "headers": map[string]any{"Authorization": "new-test-token"},
		"command": "new-command", "args": []any{"--new"}, "env": map[string]any{"MODE": "changed"},
		"transport": "sse", "startup_timeout_sec": 15, "tool_timeout_sec": 45, "enabled": false,
		ManagedMetaKey: map[string]any{"vendor.example/config": map[string]any{"setting": "changed"}},
	} {
		t.Run(key+" connection", func(t *testing.T) {
			changed := cloneMap(config)
			changed[key] = value
			got, err := availabilityConfigHash(changed)
			if err != nil || got == want {
				t.Fatalf("connection edit reused stale availability: %v", err)
			}
		})
	}
}

type availabilityTestProber struct {
	probe func(context.Context, string, map[string]any) (ProbeResult, error)
}

func (p availabilityTestProber) Probe(ctx context.Context, name string, config map[string]any) (ProbeResult, error) {
	return p.probe(ctx, name, config)
}

func TestListAvailableServersFiltersUnavailableRemoteAndKeepsStoredConfig(t *testing.T) {
	store := &memoryServerStore{servers: map[string]any{
		"local": map[string]any{"url": "https://local.example.test/mcp"},
		"allowed": RemoteServer{
			ID: "remote-allowed", Name: "allowed", URL: "https://allowed.example.test/mcp",
		}.Config(),
		"denied": RemoteServer{
			ID: "remote-denied", Name: "denied", URL: "https://denied.example.test/mcp",
		}.Config(),
	}}
	svc := NewService(
		WithServerStore(store),
		WithServerProber(availabilityTestProber{probe: func(_ context.Context, name string, _ map[string]any) (ProbeResult, error) {
			if name == "allowed" {
				return ProbeResult{Connected: true}, nil
			}
			return ProbeResult{}, errors.New("access denied")
		}}),
	)

	available, err := svc.ListAvailableServers(context.Background())
	if err != nil {
		t.Fatalf("ListAvailableServers() error = %v", err)
	}
	for _, name := range []string{"local", "allowed"} {
		if _, ok := available[name]; !ok {
			t.Fatalf("available servers = %#v, want %q", available, name)
		}
	}
	if _, ok := available["denied"]; ok {
		t.Fatalf("available servers retained denied remote: %#v", available)
	}

	stored, err := svc.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers() error = %v", err)
	}
	if _, ok := stored["denied"]; !ok {
		t.Fatalf("stored servers removed denied remote: %#v", stored)
	}
}

func TestListAvailableServersWithStatusReportsPendingProbeUntilCompletion(t *testing.T) {
	release := make(chan struct{})
	svc := NewService(
		WithServerStore(&memoryServerStore{servers: map[string]any{
			"slow": RemoteServer{ID: "remote-slow", Name: "slow", URL: "https://slow.example.test/mcp"}.Config(),
		}}),
		WithServerProber(availabilityTestProber{probe: func(ctx context.Context, _ string, _ map[string]any) (ProbeResult, error) {
			select {
			case <-release:
				return ProbeResult{Connected: true}, nil
			case <-ctx.Done():
				return ProbeResult{}, ctx.Err()
			}
		}}),
	)

	servers, pending, err := svc.ListAvailableServersWithStatus(context.Background())
	if err != nil {
		t.Fatalf("ListAvailableServersWithStatus() error = %v", err)
	}
	if !pending {
		t.Fatal("ListAvailableServersWithStatus() pending = false, want true")
	}
	if _, ok := servers["slow"]; ok {
		t.Fatalf("pending server was exposed before its probe completed: %#v", servers)
	}

	close(release)
	servers, pending, err = svc.ListAvailableServersWithStatus(context.Background())
	if err != nil {
		t.Fatalf("ListAvailableServersWithStatus() after completion error = %v", err)
	}
	if pending {
		t.Fatal("ListAvailableServersWithStatus() pending = true after completion")
	}
	if _, ok := servers["slow"]; !ok {
		t.Fatalf("completed healthy server missing from catalog: %#v", servers)
	}
}

func TestRequireServerAvailableRejectsFailedRemoteProbe(t *testing.T) {
	svc := NewService(WithServerProber(availabilityTestProber{probe: func(context.Context, string, map[string]any) (ProbeResult, error) {
		return ProbeResult{}, errors.New("forbidden")
	}}))
	config := RemoteServer{ID: "remote-denied", Name: "denied", URL: "https://denied.example.test/mcp"}.Config()

	err := svc.RequireServerAvailable(context.Background(), "denied", config)
	if !errors.Is(err, ErrServerUnavailable) {
		t.Fatalf("RequireServerAvailable() error = %v, want ErrServerUnavailable", err)
	}
}

func TestAvailabilityChecksHonorConfiguredProbeTimeouts(t *testing.T) {
	for _, test := range []struct {
		name  string
		check func(*Service, map[string]any) bool
	}{
		{
			name: "agent binding",
			check: func(svc *Service, config map[string]any) bool {
				return svc.RequireServerAvailable(context.Background(), "slow", config) == nil
			},
		},
		{
			name: "template creation",
			check: func(svc *Service, config map[string]any) bool {
				filtered, skipped := svc.FilterAvailableTemplateServers(context.Background(), map[string]any{"slow": config})
				_, retained := filtered["slow"]
				return retained && len(skipped) == 0
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := NewService(WithServerProber(availabilityTestProber{probe: func(ctx context.Context, _ string, _ map[string]any) (ProbeResult, error) {
				select {
				case <-time.After(1100 * time.Millisecond):
					return ProbeResult{Connected: true}, nil
				case <-ctx.Done():
					return ProbeResult{}, ctx.Err()
				}
			}}))
			config := RemoteServer{ID: "remote-slow", Name: "slow", URL: "https://slow.example.test/mcp"}.Config()
			config["startup_timeout_sec"] = 2
			config["tool_timeout_sec"] = 2
			if !test.check(svc, config) {
				t.Fatal("availability check rejected a server that completed within its configured timeouts")
			}
		})
	}
}

func TestRemoteServerConfigCarriesStableHubIdentity(t *testing.T) {
	config := RemoteServer{ID: "builtin:calendar", Name: "calendar", URL: "https://mcp.example.test/calendar"}.Config()
	if !IsRemoteHubServer(config) {
		t.Fatalf("IsRemoteHubServer(%#v) = false, want true", config)
	}
	meta := config[ManagedMetaKey].(map[string]any)[ManagedMetaNamespace].(map[string]any)
	if meta["resource_id"] != "builtin:calendar" || meta["name"] != "calendar" {
		t.Fatalf("remote metadata = %#v", meta)
	}
}

func TestFilterAvailableTemplateServersSkipsUnavailableManagedResourcesOnly(t *testing.T) {
	svc := NewService(WithServerProber(availabilityTestProber{probe: func(_ context.Context, name string, _ map[string]any) (ProbeResult, error) {
		if name == "allowed" {
			return ProbeResult{Connected: true}, nil
		}
		return ProbeResult{}, errors.New("forbidden")
	}}))
	servers := map[string]any{
		"manual":  map[string]any{"url": "https://third-party.example.test/mcp"},
		"allowed": RemoteServer{ID: "remote-allowed", Name: "Allowed MCP", URL: "https://allowed.example.test/mcp"}.Config(),
		"denied":  RemoteServer{ID: "remote-denied", Name: "Denied MCP", URL: "https://denied.example.test/mcp"}.Config(),
		"wiki-content-42": map[string]any{
			"url":         "https://wiki.example.test/mcp",
			"description": "Product Handbook | Internal docs",
			"_meta": map[string]any{"com.opencsg/mcp": map[string]any{
				"type": "llm_wiki", "resource_id": "7", "content_id": "wiki-content-42", "auth_type": "csghub_access_token",
			}},
		},
	}

	filtered, skipped := svc.FilterAvailableTemplateServers(context.Background(), servers)
	for _, name := range []string{"manual", "allowed"} {
		if _, ok := filtered[name]; !ok {
			t.Fatalf("filtered servers = %#v, want %q", filtered, name)
		}
	}
	for _, name := range []string{"denied", "wiki-content-42"} {
		if _, ok := filtered[name]; ok {
			t.Fatalf("filtered servers retained unavailable %q: %#v", name, filtered)
		}
	}
	if len(skipped) != 2 || skipped[0].Type != TemplateResourceKnowledgeBase || skipped[0].Name != "Product Handbook" || skipped[1].Type != TemplateResourceMCP || skipped[1].Name != "Denied MCP" {
		t.Fatalf("skipped resources = %#v", skipped)
	}
	if _, ok := servers["denied"]; !ok {
		t.Fatalf("input template servers were mutated: %#v", servers)
	}
}

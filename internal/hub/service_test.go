package hub

import (
	"context"
	"errors"
	"testing"

	"csgclaw/internal/config"
)

func TestNewServiceUsesResolvedBuiltinRegistry(t *testing.T) {
	var got []config.HubRegistryConfig
	svc, err := NewService(config.HubConfig{}, func(cfg config.HubRegistryConfig) (Store, error) {
		got = append(got, cfg)
		return stubStore{}, nil
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if svc == nil {
		t.Fatal("NewService() = nil, want service")
	}
	if len(got) != 3 {
		t.Fatalf("len(factory calls) = %d, want 3", len(got))
	}
	if got[0].Name != config.DefaultHubRegistry {
		t.Fatalf("factory registry name = %q, want %q", got[0].Name, config.DefaultHubRegistry)
	}
	if got[0].Kind != config.HubRegistryKindBuiltin {
		t.Fatalf("factory registry kind = %q, want %q", got[0].Kind, config.HubRegistryKindBuiltin)
	}
	if got[1].Name != config.DefaultHubPublishRegistry {
		t.Fatalf("factory registry name = %q, want %q", got[1].Name, config.DefaultHubPublishRegistry)
	}
	if got[1].Kind != config.HubRegistryKindLocal {
		t.Fatalf("factory registry kind = %q, want %q", got[1].Kind, config.HubRegistryKindLocal)
	}
	if got[2].Name != config.DefaultOfficialHubRegistryName {
		t.Fatalf("factory registry name = %q, want %q", got[2].Name, config.DefaultOfficialHubRegistryName)
	}
	if got[2].Kind != config.HubRegistryKindRemote {
		t.Fatalf("factory registry kind = %q, want %q", got[2].Kind, config.HubRegistryKindRemote)
	}
}

func TestListAggregatesAndNamespacesTemplates(t *testing.T) {
	svc := mustService(t, config.HubConfig{
		DefaultRegistry:        "team",
		DefaultPublishRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "builtin", Kind: RegistryKindBuiltin, Enabled: true},
			{Name: "team", Kind: RegistryKindRemote, Enabled: true},
		},
	}, map[string]Store{
		"builtin": stubStore{
			listResult: []Template{{ID: "frontend-alice", Name: "frontend-alice"}},
		},
		"local": stubStore{},
		"team": stubStore{
			listResult: []Template{{Name: "review-bot"}},
		},
	})

	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), 2; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := items[0].ID, "builtin.frontend-alice"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}
	if got, want := items[0].Source.Name, "builtin"; got != want {
		t.Fatalf("List()[0].Source.Name = %q, want %q", got, want)
	}
	if got, want := items[1].ID, "team.review-bot"; got != want {
		t.Fatalf("List()[1].ID = %q, want %q", got, want)
	}
	if got, want := items[1].Source.Kind, RegistryKindRemote; got != want {
		t.Fatalf("List()[1].Source.Kind = %q, want %q", got, want)
	}
}

func TestGetUsesDefaultRegistryForUnqualifiedID(t *testing.T) {
	teamStore := &recordingStore{
		getResult: Template{ID: "frontend-alice", Name: "frontend-alice"},
	}
	svc := mustService(t, config.HubConfig{
		DefaultRegistry:        "team",
		DefaultPublishRegistry: "team",
		Registries: []config.HubRegistryConfig{
			{Name: "team", Kind: RegistryKindRemote, Enabled: true},
		},
	}, map[string]Store{
		"team": teamStore,
	})

	item, err := svc.Get(context.Background(), "frontend-alice")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got, want := teamStore.lastGetID, "frontend-alice"; got != want {
		t.Fatalf("store Get id = %q, want %q", got, want)
	}
	if got, want := item.ID, "team.frontend-alice"; got != want {
		t.Fatalf("Get().ID = %q, want %q", got, want)
	}
}

func TestGetUsesQualifiedRegistryWhenPresent(t *testing.T) {
	localStore := &recordingStore{
		getResult: Template{ID: "frontend-alice", Name: "frontend-alice"},
	}
	svc := mustService(t, config.HubConfig{
		DefaultRegistry:        "team",
		DefaultPublishRegistry: "team",
		Registries: []config.HubRegistryConfig{
			{Name: "local", Kind: RegistryKindLocal, Enabled: true},
			{Name: "team", Kind: RegistryKindRemote, Enabled: true},
		},
	}, map[string]Store{
		"local": localStore,
		"team":  stubStore{},
	})

	item, err := svc.Get(context.Background(), "local.frontend-alice")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got, want := localStore.lastGetID, "frontend-alice"; got != want {
		t.Fatalf("store Get id = %q, want %q", got, want)
	}
	if got, want := item.Source.Name, "local"; got != want {
		t.Fatalf("Get().Source.Name = %q, want %q", got, want)
	}
}

func TestGetUsesLongestQualifiedRegistryPrefix(t *testing.T) {
	regionalStore := &recordingStore{
		getResult: Template{ID: "review.bot", Name: "review.bot"},
	}
	svc := mustService(t, config.HubConfig{
		DefaultRegistry:        "team",
		DefaultPublishRegistry: "team",
		Registries: []config.HubRegistryConfig{
			{Name: "team", Kind: RegistryKindRemote, Enabled: true},
			{Name: "team.us", Kind: RegistryKindRemote, Enabled: true},
		},
	}, map[string]Store{
		"team":    stubStore{},
		"team.us": regionalStore,
	})

	item, err := svc.Get(context.Background(), "team.us.review.bot")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got, want := regionalStore.lastGetID, "review.bot"; got != want {
		t.Fatalf("store Get id = %q, want %q", got, want)
	}
	if got, want := item.ID, "team.us.review.bot"; got != want {
		t.Fatalf("Get().ID = %q, want %q", got, want)
	}
}

func TestPublishUsesDefaultPublishRegistry(t *testing.T) {
	localStore := &recordingStore{
		publishResult: Template{ID: "frontend-alice", Name: "frontend-alice"},
	}
	svc := mustService(t, config.HubConfig{
		DefaultRegistry:        "builtin",
		DefaultPublishRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "builtin", Kind: RegistryKindBuiltin, Enabled: true},
			{Name: "local", Kind: RegistryKindLocal, Enabled: true},
		},
	}, map[string]Store{
		"builtin": stubStore{},
		"local":   localStore,
	})

	item, err := svc.Publish(context.Background(), PublishSpec{Name: "frontend-alice", Role: TemplateRoleWorker})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got, want := localStore.lastPublishSpec.Registry, "local"; got != want {
		t.Fatalf("publish registry = %q, want %q", got, want)
	}
	if got, want := item.ID, "local.frontend-alice"; got != want {
		t.Fatalf("Publish().ID = %q, want %q", got, want)
	}
}

func TestListContinuesWhenRegistryFails(t *testing.T) {
	svc := mustService(t, config.HubConfig{
		DefaultRegistry:        "builtin",
		DefaultPublishRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "builtin", Kind: RegistryKindBuiltin, Enabled: true},
			{Name: config.DefaultOfficialHubRegistryName, Kind: RegistryKindRemote, Enabled: true},
		},
	}, map[string]Store{
		"builtin": stubStore{
			listResult: []Template{{ID: "picoclaw-manager", Name: "picoclaw-manager"}},
		},
		config.DefaultOfficialHubRegistryName: stubStore{listErr: errors.New("network down")},
	})

	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := len(items), 1; got != want {
		t.Fatalf("len(List()) = %d, want %d", got, want)
	}
	if got, want := items[0].ID, "builtin.picoclaw-manager"; got != want {
		t.Fatalf("List()[0].ID = %q, want %q", got, want)
	}
}

func TestDeleteRemovesLocalTemplate(t *testing.T) {
	registryRoot := t.TempDir()
	store := NewLocalStore(registryRoot)
	if _, err := store.Publish(context.Background(), PublishSpec{
		ID:          "review-bot",
		Name:        "review-bot",
		Role:        TemplateRoleWorker,
		RuntimeKind: "picoclaw_sandbox",
		Image:       "agent-image:test",
	}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	svc, err := NewService(config.HubConfig{
		DefaultRegistry: "local",
		Registries: []config.HubRegistryConfig{
			{Name: "local", Kind: RegistryKindLocal, Path: registryRoot, Enabled: true},
		},
	}, DefaultStoreFactory)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := svc.Delete(context.Background(), "local.review-bot"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Get(context.Background(), "local.review-bot"); err == nil {
		t.Fatal("Get() after Delete() succeeded, want error")
	}
}

func TestDeleteRejectsBuiltinRegistry(t *testing.T) {
	svc := mustService(t, config.HubConfig{
		DefaultRegistry: "builtin",
		Registries: []config.HubRegistryConfig{
			{Name: "builtin", Kind: RegistryKindBuiltin, Enabled: true},
		},
	}, map[string]Store{
		"builtin": stubStore{},
	})

	err := svc.Delete(context.Background(), "builtin.picoclaw-worker")
	if !errors.Is(err, ErrRegistryNotDeletable) {
		t.Fatalf("Delete() error = %v, want ErrRegistryNotDeletable", err)
	}
}

func TestPublishRejectsBuiltinRegistry(t *testing.T) {
	svc := mustService(t, config.HubConfig{
		DefaultRegistry:        "builtin",
		DefaultPublishRegistry: "builtin",
		Registries: []config.HubRegistryConfig{
			{Name: "builtin", Kind: RegistryKindBuiltin, Enabled: true},
		},
	}, map[string]Store{
		"builtin": stubStore{},
	})

	_, err := svc.Publish(context.Background(), PublishSpec{Name: "frontend-alice", Role: TemplateRoleWorker})
	if !errors.Is(err, ErrRegistryNotWritable) {
		t.Fatalf("Publish() error = %v, want ErrRegistryNotWritable", err)
	}
}

type stubStore struct {
	listResult []Template
	listErr    error
	getResult  Template
}

func (s stubStore) List(context.Context) ([]Template, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]Template(nil), s.listResult...), nil
}

func (s stubStore) Get(context.Context, string) (Template, error) {
	return s.getResult, nil
}

func (s stubStore) FetchWorkspace(context.Context, string) (WorkspaceRef, error) {
	return WorkspaceRef{Kind: WorkspaceKindDir, Path: "/tmp/workspace"}, nil
}

func (s stubStore) Publish(context.Context, PublishSpec) (Template, error) {
	return Template{}, nil
}

func (s stubStore) Delete(context.Context, string) error {
	return nil
}

type recordingStore struct {
	lastGetID       string
	lastPublishSpec PublishSpec
	getResult       Template
	publishResult   Template
}

func (s *recordingStore) List(context.Context) ([]Template, error) {
	return nil, nil
}

func (s *recordingStore) Get(_ context.Context, id string) (Template, error) {
	s.lastGetID = id
	return s.getResult, nil
}

func (s *recordingStore) FetchWorkspace(context.Context, string) (WorkspaceRef, error) {
	return WorkspaceRef{}, nil
}

func (s *recordingStore) Publish(_ context.Context, spec PublishSpec) (Template, error) {
	s.lastPublishSpec = spec
	return s.publishResult, nil
}

func (s *recordingStore) Delete(context.Context, string) error {
	return nil
}

func mustService(t *testing.T, cfg config.HubConfig, stores map[string]Store) *Service {
	t.Helper()
	svc, err := NewService(cfg, func(reg config.HubRegistryConfig) (Store, error) {
		if store, ok := stores[reg.Name]; ok {
			return store, nil
		}
		return stubStore{}, nil
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

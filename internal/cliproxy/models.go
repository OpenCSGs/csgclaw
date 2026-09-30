package cliproxy

import (
	"context"
	"sync"

	"csgclaw/internal/modelcap"

	cliproxysdk "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
)

// The v8.0.4 SDK embeds an older catalog and does not start the standalone
// server's remote catalog updater. Supplement the two newer entries published
// at github.com/router-for-me/models until an SDK release embeds them.
// Requiring the preceding model preserves the SDK's subscription filtering.
var supplementalModels = []struct {
	provider, requires, id, displayName string
	created                             int64
}{
	{ProviderCodex, "gpt-6-sol", "gpt-6.1-sol", "GPT 6.1 Sol", 1790640000},
	{"claude", "claude-sonnet-5", "claude-sonnet-5-5", "Claude Sonnet 5.5", 1790553600},
}

type modelCatalog struct {
	mu      sync.Mutex
	manager *coreauth.Manager
	stopped bool
}

func (c *modelCatalog) OnModelsRegistered(ctx context.Context, provider, clientID string, _ []*cliproxysdk.ModelInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reconcileClient(ctx, provider, clientID)
}

func (*modelCatalog) OnModelsUnregistered(context.Context, string, string) {}

func (c *modelCatalog) reconcile(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.manager == nil {
		return
	}
	for _, auth := range c.manager.List() {
		c.reconcileClient(ctx, auth.Provider, auth.ID)
	}
}

func catalogComplete(provider string, models []string) bool {
	for _, entry := range supplementalModels {
		if provider == entry.provider && containsModel(models, entry.requires) && !containsModel(models, entry.id) {
			return false
		}
	}
	return true
}

func containsModel(models []string, id string) bool {
	for _, model := range models {
		if model == id {
			return true
		}
	}
	return false
}

func (c *modelCatalog) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
}

// Read the current registry instead of the asynchronous hook's snapshot, so a
// delayed callback cannot restore models removed by a newer auth update.
func (c *modelCatalog) reconcileClient(ctx context.Context, provider, clientID string) {
	if c.stopped || c.manager == nil || ctx.Err() != nil {
		return
	}
	auth, ok := c.manager.GetByID(clientID)
	if !ok || auth.Disabled || auth.Provider != provider {
		return
	}
	registry := cliproxysdk.GlobalModelRegistry()
	models := registry.GetModelsForClient(clientID)
	byID := make(map[string]*cliproxysdk.ModelInfo, len(models))
	for _, model := range models {
		byID[model.ID] = model
	}
	changed := false
	for _, entry := range supplementalModels {
		if provider != entry.provider || byID[entry.id] != nil || byID[entry.requires] == nil {
			continue
		}
		model := *byID[entry.requires]
		model.ID = entry.id
		model.DisplayName = entry.displayName
		model.Created = entry.created
		model.MetadataModelID = ""
		model.Name = ""
		if model.Thinking != nil {
			thinking := *model.Thinking
			thinking.Levels = []string{"low", "medium", "high", "xhigh", "max"}
			if provider == ProviderCodex {
				thinking.ZeroAllowed = false
				thinking.DynamicAllowed = false
			}
			model.Thinking = &thinking
		}
		if provider == ProviderCodex {
			model.Description = "Latest workhorse model for coding and everyday work."
			// Advertise the model's full reference capacity consistently with
			// the CSGClaw profile, rather than the client catalog's default.
			window := modelcap.Resolve(provider, "", entry.id, modelcap.Metadata{}, modelcap.Metadata{}).ContextWindow
			model.ContextLength = int(window)
			model.MaxContextLength = int(window)
			model.SupportConfigurationUpdate = true
		} else if model.NativeCapabilities == nil {
			// Sonnet 5.5 adds native web search, as does the embedded Opus 5.5.
			if opus := byID["claude-opus-5-5"]; opus != nil {
				model.NativeCapabilities = opus.NativeCapabilities
			}
		}
		models = append(models, &model)
		changed = true
	}
	if !changed {
		return
	}
	registry.RegisterClient(clientID, provider, models)
	// An auth removal racing registration must not leave a visible model client.
	if latest, exists := c.manager.GetByID(clientID); !exists || latest.Disabled || latest.Provider != provider {
		registry.UnregisterClient(clientID)
		return
	}
	c.manager.RefreshSchedulerEntry(clientID)
}

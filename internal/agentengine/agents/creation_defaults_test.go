package agents

import (
	"context"
	"csgclaw/internal/config"
	"csgclaw/internal/modelprovider"
	agentruntime "csgclaw/internal/runtime"
	"errors"
	"path/filepath"
	"testing"
)

func TestCreationRemembersAllModelsAndIgnoresEditsAndRefresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	state := filepath.Join(t.TempDir(), "agents.json")
	llm := config.LLMConfig{Default: "service.chat", Providers: map[string]config.ProviderConfig{"service": {BaseURL: "https://model.example/v1", APIKey: "test-key", Models: []string{"chat", "other"}, ImageModels: []string{"image"}, VideoModels: []string{"video"}}}}
	rt := fakeAgentRuntime{kind: RuntimeKindCodex, new: func(_ context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
		if spec.AgentName == "failed" {
			return agentruntime.Handle{}, errors.New("creation failed")
		}
		return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: "handle"}, nil
	}}
	svc, err := NewControllerWithLLM(llm, config.ServerConfig{}, "manager:test", state, WithRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	selection := AgentProfile{Provider: ProviderAPI, ModelProviderID: "service", ModelID: "chat", ImageGeneration: &modelprovider.ImageGenerationConfig{ProviderID: "service", ModelID: "image"}, VideoGeneration: &modelprovider.VideoGenerationConfig{ProviderID: "service", ModelID: "video"}}
	created, err := svc.CreateWorker(context.Background(), CreateAgentSpec{ID: "u-created", Name: "created", RuntimeKind: RuntimeKindCodex, Profile: "service.chat", AgentProfile: selection})
	if err != nil {
		t.Fatal(err)
	}
	if created.AgentProfile.ImageGeneration == nil || created.AgentProfile.VideoGeneration == nil {
		t.Fatal("creation selector discarded media selections")
	}
	check := func(s *Controller) {
		t.Helper()
		got := s.Models().ProfileDefaultsView()
		if got.ModelID != "chat" || got.ImageGeneration == nil || got.ImageGeneration.ModelID != "image" || got.VideoGeneration == nil || got.VideoGeneration.ModelID != "video" {
			t.Fatalf("remembered creation selections changed: %+v", got)
		}
	}
	check(svc)
	edited := AgentProfile{Provider: ProviderAPI, ModelProviderID: "service", ModelID: "other"}
	if _, err := svc.UpdateAgentProfile(created.ID, edited); err != nil {
		t.Fatal(err)
	}
	check(svc)
	edited.ModelID = "chat"
	if _, err := svc.UpdateRecord(context.Background(), created.ID, UpdateRequest{AgentProfile: &edited}); err != nil {
		t.Fatal(err)
	}
	check(svc)
	llm.Default = "service.other"
	svc.Models().SetLLMConfig(llm)
	check(svc)
	if _, err := svc.CreateWorker(context.Background(), CreateAgentSpec{ID: "u-failed", Name: "failed", RuntimeKind: RuntimeKindCodex, Profile: "service.other", AgentProfile: edited}); err == nil {
		t.Fatal("fixture creation should fail")
	}
	check(svc)
	reloaded, err := NewControllerWithLLM(llm, config.ServerConfig{}, "manager:test", state, WithRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	check(reloaded)
}

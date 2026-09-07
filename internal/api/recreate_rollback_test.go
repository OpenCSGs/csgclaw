package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
	agentruntime "csgclaw/internal/runtime"
	"csgclaw/internal/sandbox"
	"csgclaw/internal/sandbox/sandboxtest"
)

type recreateExtensionObserver struct{ fail bool }

func (*recreateExtensionObserver) PrepareRuntime(context.Context, string) error { return nil }
func (o *recreateExtensionObserver) RuntimeStarted(context.Context, string) error {
	if o.fail {
		return errors.New("injected extension observation failure")
	}
	return nil
}
func (*recreateExtensionObserver) RuntimeStopped(context.Context, string) error   { return nil }
func (*recreateExtensionObserver) DeleteExtensions(context.Context, string) error { return nil }
func (*recreateExtensionObserver) RuntimeReady(string) error                      { return nil }

// Exercise the HTTP recreation path through the real Engine/Controller and an
// in-memory container. Every failure is injected after New returns a live handle.
func TestRecreateAPIRemovesUncommittedReplacement(t *testing.T) {
	for _, kind := range []string{agent.RuntimeKindCodex, agent.RuntimeKindOpenClawSandbox, agent.RuntimeKindPicoClawSandbox} {
		for _, failure := range []string{"observation", "skills", "info", "persist", "canceled", "cleanup", "none"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				containers := startedNativeSandbox{sandboxtest.NewRuntime()}
				old, err := containers.Create(ctx, sandbox.CreateSpec{Name: "old"})
				if err != nil {
					t.Fatal(err)
				}
				oldInfo, err := old.Info(ctx)
				if err != nil {
					t.Fatal(err)
				}
				target := completeWorkerAgent("agent-recreate", "recreate")
				target.RuntimeKind = kind
				target.BoxID = oldInfo.ID
				var statePath, skillsRoot, newID string
				cleanupCalled := false
				rt := fakeCompatRuntime{kind: kind}
				rt.new = func(ctx context.Context, spec agentruntime.Spec) (agentruntime.Handle, error) {
					created, err := containers.Create(ctx, sandbox.CreateSpec{Name: "replacement"})
					if err != nil {
						return agentruntime.Handle{}, err
					}
					info, err := created.Info(ctx)
					if err != nil {
						return agentruntime.Handle{}, err
					}
					newID = info.ID
					if failure == "skills" {
						if err := os.Rename(skillsRoot, skillsRoot+".saved"); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(skillsRoot, []byte("blocks restoration"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					if failure == "canceled" {
						cancel()
					}
					return agentruntime.Handle{RuntimeID: spec.RuntimeID, HandleID: newID}, nil
				}
				rt.info = func(ctx context.Context, handle agentruntime.Handle) (agentruntime.Info, error) {
					if newID != "" && handle.HandleID == newID {
						if failure == "info" || failure == "cleanup" {
							return agentruntime.Info{}, errors.New("injected replacement info failure")
						}
						if err := ctx.Err(); err != nil {
							return agentruntime.Info{}, err
						}
						if failure == "persist" {
							if err := os.Rename(statePath, statePath+".saved"); err != nil {
								t.Fatal(err)
							}
							if err := os.Mkdir(statePath, 0700); err != nil {
								t.Fatal(err)
							}
						}
					}
					instance, err := containers.Get(ctx, handle.HandleID)
					if err != nil {
						return agentruntime.Info{}, err
					}
					info, err := instance.Info(ctx)
					return agentruntime.Info{HandleID: info.ID, State: agentruntime.State(info.State)}, err
				}
				rt.del = func(ctx context.Context, handle agentruntime.Handle) error {
					if newID != "" && handle.HandleID == newID {
						cleanupCalled = true
						if err := ctx.Err(); err != nil {
							t.Errorf("cleanup inherited canceled request: %v", err)
							return err
						}
						if _, bounded := ctx.Deadline(); !bounded {
							t.Error("cleanup has no deadline")
						}
						if failure == "cleanup" {
							return errors.New("injected replacement cleanup failure")
						}
					}
					return containers.Remove(ctx, handle.HandleID, sandbox.RemoveOptions{Force: true})
				}
				controller, path := mustNewSeededServiceWithPathAndOptions(t, []agent.Agent{target}, agent.WithRuntime(rt))
				statePath = path
				layout, err := controller.AgentLayout(target.ID)
				if err != nil {
					t.Fatal(err)
				}
				skillsRoot = layout.SkillsRoot
				if err := os.MkdirAll(filepath.Join(skillsRoot, "user-skill"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(skillsRoot, "user-skill", "SKILL.md"), []byte("user content"), 0600); err != nil {
					t.Fatal(err)
				}
				engine := agentengine.New(controller)
				controller.AttachEngine(nil, nil, &recreateExtensionObserver{fail: failure == "observation"})
				h := NewHandler(AgentServices{Records: controller, Workspace: controller.Workspace(), Models: controller.Models(), Runtime: controller}, engine, nil, nil, nil, nil, nil)
				w := httptest.NewRecorder()
				h.Routes().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-recreate/recreate", nil).WithContext(ctx))
				if newID == "" {
					t.Fatalf("failure did not reach Runtime.New: HTTP %d %s", w.Code, w.Body)
				}
				_, liveErr := containers.Get(context.Background(), newID)
				if failure == "none" {
					if w.Code != http.StatusOK || cleanupCalled || liveErr != nil {
						t.Fatalf("committed Runtime was rolled back: HTTP %d cleanup=%v err=%v", w.Code, cleanupCalled, liveErr)
					}
					return
				}
				if w.Code == http.StatusOK {
					t.Fatalf("post-New failure was acknowledged: %s", w.Body)
				}
				if failure == "cleanup" {
					if !cleanupCalled || !strings.Contains(w.Body.String(), "injected replacement info failure") || !strings.Contains(w.Body.String(), "injected replacement cleanup failure") {
						t.Fatalf("original and cleanup failures were not both reported: cleanup=%v HTTP %d %s", cleanupCalled, w.Code, w.Body)
					}
				} else if !cleanupCalled || !sandbox.IsNotFound(liveErr) {
					t.Fatalf("HTTP %d left replacement %q alive; cleanup=%v err=%v", w.Code, newID, cleanupCalled, liveErr)
				}
				if failure == "info" && !strings.Contains(w.Body.String(), "injected replacement info failure") {
					t.Fatalf("lost original failure: %s", w.Body)
				}
				if failure != "persist" {
					if err := controller.Reload(); err != nil {
						t.Fatal(err)
					}
				}
				stored, err := engine.Agents().Get(context.Background(), target.ID, agentengine.AgentGetOptions{})
				if err != nil || stored.Status.State != agentengine.AgentStateFailed || stored.Status.SandboxID != newID || stored.Spec.Name != target.Name {
					t.Fatalf("replacement is not discoverable for retry: %+v %v", stored, err)
				}
			})
		}
	}
}

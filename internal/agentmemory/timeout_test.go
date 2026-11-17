package agentmemory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestMemoryExtractionAllowsSlowModelsAndBoundsStalledCalls(t *testing.T) {
	for _, history := range []bool{false, true} {
		name := "turn"
		if history {
			name = "history"
		}
		t.Run(name, func(t *testing.T) {
			for _, stalled := range []bool{false, true} {
				name := "slow_success"
				if stalled {
					name = "stalled_timeout"
				}
				t.Run(name, func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						store := openTestStore(t, t.TempDir(), "local")
						if _, err := store.GetOrInitialize(testScope); err != nil {
							t.Fatal(err)
						}
						worker := Worker{Store: store}
						if history {
							worker.History = historyOnce("source", "Use concise Chinese reports")
						} else {
							worker.process(t.Context(), testScope)
							enqueueMemory(t, store, "source", "Use concise Chinese reports")
						}
						worker.Extract = func(ctx context.Context, _ string, input Extraction) ([]Change, error) {
							select {
							case <-ctx.Done():
								return nil, errors.New("Cannot reach the model service")
							case <-time.After(6 * time.Minute):
								if stalled {
									<-ctx.Done()
									return nil, errors.New("Cannot reach the model service")
								}
								return []Change{{Kind: "preference", Text: "Use concise Chinese reports", SourceMessageID: input.Messages[0].MessageID}}, nil
							}
						}
						started := time.Now()
						worker.process(t.Context(), testScope)
						doc := getMemory(t, store, testScope)
						if stalled {
							if time.Since(started) != 10*time.Minute || doc.LastError == nil || !strings.Contains(doc.LastError.Message, "timed out") || doc.Status != "pending" {
								t.Fatalf("stalled call must time out and remain retryable: elapsed=%s document=%+v", time.Since(started), doc)
							}
							worker.Extract = func(_ context.Context, _ string, input Extraction) ([]Change, error) {
								return []Change{{Kind: "preference", Text: "Use concise Chinese reports", SourceMessageID: input.Messages[0].MessageID}}, nil
							}
							time.Sleep(15 * time.Second)
							worker.process(t.Context(), testScope)
							worker.process(t.Context(), testScope)
							recovered := getMemory(t, store, testScope)
							if recovered.LastError != nil || len(recovered.Entries) != 1 || recovered.Status != "idle" {
								t.Fatalf("retry did not recover: %+v", recovered)
							}
						} else {
							if time.Since(started) != 6*time.Minute || len(doc.Entries) != 1 || doc.LastError != nil {
								t.Fatalf("slow successful call was interrupted: elapsed=%s document=%+v", time.Since(started), doc)
							}
						}
					})
				})
			}
		})
	}
}

func TestSlowMemoryCanBeClearedAndShutdownResumesItsQueue(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		name := "clear"
		if shutdown {
			name = "shutdown"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				store := openTestStore(t, t.TempDir(), "local")
				if _, err := store.GetOrInitialize(testScope); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan struct{})
				worker := Worker{Store: store, History: historyOnce("source", "Keep concise reports"), Extract: func(ctx context.Context, _ string, _ Extraction) ([]Change, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}}
				started := time.Now()
				go func() { defer close(done); worker.process(ctx, testScope) }()
				synctest.Wait()
				if shutdown {
					cancel()
				} else {
					doc := getMemory(t, store, testScope)
					empty := []Entry{}
					if _, err := store.Update(testScope, Update{Revision: &doc.Revision, Entries: &empty}); err != nil {
						t.Fatal(err)
					}
				}
				<-done
				doc := getMemory(t, store, testScope)
				if time.Since(started) != 0 || doc.LastError != nil || len(doc.Entries) != 0 {
					t.Fatalf("cancellation did not stop immediately: %+v", doc)
				}
				if shutdown {
					if doc.Status != "pending" {
						t.Fatalf("queue was lost on shutdown: %+v", doc)
					}
					worker.Extract = func(_ context.Context, _ string, input Extraction) ([]Change, error) {
						return []Change{{Kind: "preference", Text: "Keep concise reports", SourceMessageID: input.Messages[0].MessageID}}, nil
					}
					restored := openTestStore(t, store.root, "local")
					worker.Store = restored
					worker.process(t.Context(), testScope)
					if len(getMemory(t, restored, testScope).Entries) != 1 {
						t.Fatal("interrupted extraction did not resume")
					}
				} else if doc.Status != "idle" {
					t.Fatalf("cleared work remained pending: %+v", doc)
				}
			})
		})
	}
}

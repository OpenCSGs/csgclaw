package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"csgclaw/internal/connectorcli"
	"csgclaw/internal/localstore"
)

func assertCancelledCLIResource(t *testing.T, s *Service, id, status string) {
	t.Helper()
	check := func(item Installation, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if item.Status != status || item.Authorized != (status == "connected") {
			t.Fatalf("resource status=%s authorized=%t, want %s", item.Status, item.Authorized, status)
		}
		if item.Operation == nil || item.Operation.Status != "cancelled" || item.Operation.ErrorCode != "" {
			t.Fatalf("operation=%+v", item.Operation)
		}
		if item.LastErrorCode != "" || item.LastError != "" || item.LastErrorHTTPStatus != 0 {
			t.Fatalf("cancelled resource retained error: code=%q message=%q HTTP=%d",
				item.LastErrorCode, item.LastError, item.LastErrorHTTPStatus)
		}
	}
	check(s.Get(t.Context(), "", id))
	check(s.CheckResource(t.Context(), id, true))
	items, err := s.ListResources(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].InstallationID != id {
		t.Fatalf("unexpected resources: count=%d", len(items))
	}
	check(items[0], nil)
}

func TestFeishuCLICancellationClearsResourceErrors(t *testing.T) {
	for _, stage := range []string{"app_authorization", "user_authorization"} {
		t.Run(stage, func(t *testing.T) {
			s, fixture, id := newCLIFixture(t)
			entered := make(chan struct{})
			s.options.CLI.Run = func(ctx context.Context, client connectorcli.Client, input io.Reader, args ...string) ([]byte, error) {
				cancelHere := stage == "app_authorization" && args[0] == "config" && args[1] == "init"
				cancelHere = cancelHere || (stage == "user_authorization" && args[0] == "auth" && args[1] == "login" && args[2] == "--device-code")
				if cancelHere {
					close(entered)
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return fixture.run(ctx, client, input, args...)
			}
			op := startCLITest(t, s, id, "connect", "cancel-authorization")
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("authorization did not reach the cancellation point")
			}
			if _, err := s.CancelCLI(t.Context(), id, op.SessionID); err != nil {
				t.Fatal(err)
			}
			if got := waitCLIOperation(t, s, id, op); got.Status != "cancelled" || got.ErrorCode != "" {
				t.Fatalf("cancelled operation=%+v", got)
			}
			status := "needs_configuration"
			if stage == "user_authorization" {
				status = "connected"
			}
			assertCancelledCLIResource(t, s, id, status)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			restored, err := NewService(s.path, s.options)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restored.Close() })
			assertCancelledCLIResource(t, restored, id, status)
		})
	}
}

func TestLoadResourcesRepairsCancelledCLIErrors(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        string
		settled       bool
		operationCode string
		resourceCode  string
		repair        bool
	}{
		{name: "cancelled", status: "cancelled", settled: true, repair: true},
		{name: "unsettled", status: "cancelled"},
		{name: "operation_error", status: "cancelled", settled: true, operationCode: "cleanup_pending"},
		{name: "resource_error", status: "cancelled", settled: true, resourceCode: "migration_failed"},
		{name: "cleanup_failed", status: "failed", operationCode: "cleanup_pending", resourceCode: "cleanup_pending"},
		{name: "authorization_expired", status: "expired", settled: true, operationCode: "authorization_expired", resourceCode: "authorization_expired"},
		{name: "authorization_denied", status: "denied", settled: true, operationCode: "authorization_denied", resourceCode: "authorization_denied"},
		{name: "running", status: "running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, id := newCLIFixture(t)
			if got := waitCLIOperation(t, s, id, startCLITest(t, s, id, "connect", "initial")); got.Status != "completed" {
				t.Fatalf("initial operation=%+v", got)
			}
			if err := s.editCLI(id, func(r *record) error {
				r.CLI.Operation.Status = tc.status
				r.CLI.Operation.Settled = tc.settled
				r.CLI.Operation.ErrorCode = tc.operationCode
				r.CLI.Completed[r.CLI.Operation.ID] = r.CLI.Operation.CLIOperation
				r.LastErrorCode = tc.resourceCode
				r.LastError = "CLI connector operation could not be completed; retry the operation."
				r.LastErrorHTTPStatus = 502
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			readSaved := func() record {
				t.Helper()
				var saved map[string]record
				if _, err := localstore.ReadSection(s.path, installationsSection, &saved); err != nil {
					t.Fatal(err)
				}
				return saved[id]
			}
			before := readSaved()
			for restart := 0; restart < 2; restart++ {
				restored, err := NewService(s.path, s.options)
				if err != nil {
					t.Fatal(err)
				}
				item, err := restored.Get(t.Context(), "", id)
				if err != nil {
					t.Fatal(err)
				}
				if tc.repair {
					assertCancelledCLIResource(t, restored, id, "connected")
				} else if item.LastError != before.LastError || item.LastErrorCode != before.LastErrorCode || item.LastErrorHTTPStatus != before.LastErrorHTTPStatus {
					t.Fatalf("unresolved error changed: code=%q message=%q HTTP=%d",
						item.LastErrorCode, item.LastError, item.LastErrorHTTPStatus)
				}
				if err := restored.Close(); err != nil {
					t.Fatal(err)
				}
				after := readSaved()
				if tc.repair {
					if after.LastError != "" || after.LastErrorCode != "" || after.LastErrorHTTPStatus != 0 {
						t.Fatal("cancelled error repair was not saved")
					}
					after.LastError, after.LastErrorCode, after.LastErrorHTTPStatus = before.LastError, before.LastErrorCode, before.LastErrorHTTPStatus
					// Status is derived again when a resource is loaded.
					after.Status = before.Status
				}
				want, _ := json.Marshal(before)
				got, _ := json.Marshal(after)
				if !bytes.Equal(want, got) {
					t.Fatal("reload changed saved state beyond the cancelled resource error")
				}
			}
		})
	}
}

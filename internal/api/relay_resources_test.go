package api

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"csgclaw/internal/relay"
)

func TestCloudReadErrorsExplainRecoveryWithoutExposingDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name           string
		err            error
		status         int
		code, fragment string
	}{
		{"unsupported executor", relay.ErrResourcesUnsupported, 409, "executor_resources_unsupported", "Update the cloud agent"},
		{"unsupported upstream", &relay.HTTPError{Status: 409, Detail: relay.ProtocolError{Code: "executor_resources_unsupported"}}, 409, "executor_resources_unsupported", "Update the cloud agent"},
		{"endpoint", relay.ErrCloudEndpointUnavailable, 409, "cloud_endpoint_unavailable", "Check the instance"},
		{"offline", relay.ErrCloudAgentOffline, 409, "agent_offline", "Wait for it to reconnect"},
		{"permission", &relay.HTTPError{Status: 403}, 403, "cloud_resource_access_denied", "Check your account"},
		{"unknown", errors.New("private-upstream-diagnostic"), 502, "cloud_resources_unavailable", "Check the cloud service"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h := &Handler{}
			h.writeCloudReadError(w, tc.err)
			var body struct {
				Error struct{ Code, Message string }
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || body.Error.Code != tc.code || !strings.Contains(body.Error.Message, tc.fragment) {
				t.Fatalf("unexpected error %d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private-upstream-diagnostic") {
				t.Fatal("upstream diagnostic exposed")
			}
		})
	}
}

func TestCloudTaskResourceFailureExplainsReselection(t *testing.T) {
	for _, code := range []string{"resource_not_attached", "resource_disabled", "resource_unavailable", "invalid_resource_reference"} {
		if got := cloudTaskFailureText("en", code); !strings.Contains(got, "Remove or reselect") {
			t.Fatalf("missing English recovery action for %s: %s", code, got)
		}
		if got := cloudTaskFailureText("zh", code); !strings.Contains(got, "移除或重新选择") {
			t.Fatalf("missing Chinese recovery action for %s: %s", code, got)
		}
	}
}

func TestCloudTaskFailureCopyDoesNotExposeImplementationService(t *testing.T) {
	for _, locale := range []string{"zh", "en"} {
		for _, code := range []string{"execution_failed", "relay_task_result_unavailable", "input_file_unavailable"} {
			copy := cloudTaskFailureText(locale, code)
			if strings.Contains(strings.ToLower(copy), "relay") {
				t.Fatalf("technical service in user message: %s", copy)
			}
		}
	}
}

package agentengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFileInterfaceMapsToHTTPContentEndpoint(t *testing.T) {
	content := []byte("HTTP file content")
	files := NewFileStore().Scope("agent-a")
	file, err := files.Create(context.Background(), FileCreateRequest{
		Name: "报告.txt", MIMEType: "text/plain", SizeBytes: int64(len(content)),
	}, bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/agents/agent-a/files/"+file.ID+"/content" {
			http.NotFound(w, r)
			return
		}
		download, err := files.Get(r.Context(), file.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer download.Content.Close()
		w.Header().Set("Content-Type", download.Metadata.MediaType)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", download.Metadata.SizeBytes))
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": download.Metadata.Name}))
		w.Header().Set("ETag", `"`+download.Metadata.SHA256+`"`)
		_, _ = io.Copy(w, download.Content)
	}))
	defer server.Close()

	response, err := http.Get(server.URL + "/api/v1/agents/agent-a/files/" + file.ID + "/content")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	got, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	disposition, dispositionParams, dispositionErr := mime.ParseMediaType(response.Header.Get("Content-Disposition"))
	if response.StatusCode != http.StatusOK || !bytes.Equal(got, content) ||
		response.Header.Get("Content-Type") != file.MediaType || response.ContentLength != file.SizeBytes ||
		response.Header.Get("ETag") != `"`+file.SHA256+`"` || dispositionErr != nil ||
		disposition != "attachment" || dispositionParams["filename"] != file.Name {
		t.Fatalf("HTTP file response status=%d headers=%v content=%q", response.StatusCode, response.Header, got)
	}
}

func TestAgentEngineWireTypesUseStableJSONShapes(t *testing.T) {
	now := time.Date(2026, 8, 21, 3, 0, 0, 0, time.UTC)
	values := []any{
		Agent{ID: "agent-a", Spec: AgentSpec{Name: "A", Role: AgentRoleWorker, Runtime: RuntimeSpec{Adapter: "codex"}}, Status: AgentStatus{State: AgentStateRunning, Ready: true}, CreatedAt: now, UpdatedAt: now},
		TurnRequest{ID: "turn-1", ConversationKey: "conversation-1", Input: []InputPart{{Kind: InputPartFile, File: &InputFile{ID: "file-1"}}}, Admission: AdmissionRejectIfBusy},
		TurnEvent{TurnID: "turn-1", Sequence: 1, Kind: TurnEventActivityUpdate, Activity: &ActivityUpdate{ID: "plan", Kind: "plan_update", Payload: map[string]any{"step": 1}}},
		TurnResult{Status: TurnSucceeded, Output: "done", Files: []OutputFile{{OutputFileMetadata: OutputFileMetadata{ID: "file-1", Name: "report.txt", MediaType: "text/plain", SizeBytes: 3, SHA256: strings.Repeat("0", 64)}}}, Dispatched: true},
		InteractionResolution{ConversationKey: "conversation-1", InteractionID: "interaction-1", Answers: map[string]InteractionAnswer{"choice": {Values: []string{"yes"}}}},
	}
	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %T: %v", value, err)
		}
		if bytes.Contains(encoded, []byte("snapshot")) || bytes.Contains(encoded, []byte("SourcePath")) || bytes.Contains(encoded, []byte("source_path")) {
			t.Fatalf("%T leaked implementation detail: %s", value, encoded)
		}
	}
}

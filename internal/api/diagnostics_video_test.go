package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/channel"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/im"
	"csgclaw/internal/modelprovider"
)

func TestVideoDiagnosticRoutesIncludeBackgroundLifetime(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	bootstrap := im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Role: "admin"}}, Rooms: []im.Room{{ID: "room", Members: []string{im.AdminUserID}}}}
	writeJSONFile := func(path string, value any) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeJSONFile(statePath, bootstrap)
	origin := time.Now().Add(-3 * time.Minute)
	startMS, endMS := float64(0), float64(9000)
	snapshot := diagnostics.Snapshot{ID: "diagnostic-video", RoomID: "room", SourceID: "source", AgentID: "agent", TurnID: "turn", StartedAt: origin, Status: "succeeded", TotalMS: 9000, RuntimeStartMS: &startMS, RuntimeEndMS: &endMS}
	if err := os.MkdirAll(filepath.Join(root, "diagnostics"), 0700); err != nil {
		t.Fatal(err)
	}
	writeJSONFile(filepath.Join(root, "diagnostics", snapshot.ID+".json"), snapshot)
	svc, err := im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatal(err)
	}
	routes := (&Handler{im: svc}).Routes()
	deliver := func(state string, end string) {
		t.Helper()
		modelCompleted := ""
		if end != "" {
			modelCompleted = origin.Add(125 * time.Second).Format(time.RFC3339Nano)
		}
		_, err := svc.DeliverMessage(im.DeliverMessageRequest{RoomID: "room", SenderID: im.AdminUserID, MessageID: "video", Content: "video", Metadata: map[string]any{
			"video_generation":         contract.VideoGenerationTask{Model: &modelprovider.VideoGenerationConfig{ModelID: "video-model"}, ModelStartedAt: origin.Add(7 * time.Second).Format(time.RFC3339Nano), ModelCompletedAt: modelCompleted, ID: "job", State: state, StartedAt: origin.Add(6 * time.Second).Format(time.RFC3339Nano), EndedAt: end},
			"video_generation_context": channel.TurnContext{AgentID: "agent", RoomID: "room", SourceMessageID: "source", TurnID: "turn"},
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	get := func(path string) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		routes.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	deliver("generating", "")
	var pending diagnostics.Snapshot
	json.Unmarshal(get("/api/v1/rooms/room/diagnostics/"+snapshot.ID), &pending)
	if pending.Status != "running" || pending.TotalMS < 180000 || *pending.RuntimeEndMS != 9000 {
		t.Fatalf("pending=%+v", pending)
	}
	var page struct {
		Items []diagnostics.Snapshot `json:"items"`
	}
	json.Unmarshal(get("/api/v1/rooms/room/diagnostics?status=running"), &page)
	if len(page.Items) != 1 || len(page.Items[0].Spans) != 0 {
		t.Fatalf("page=%+v", page)
	}
	end := origin.Add(130 * time.Second)
	deliver("completed", end.Format(time.RFC3339Nano))
	var done diagnostics.Snapshot
	json.Unmarshal(get("/api/v1/rooms/room/diagnostics/"+snapshot.ID), &done)
	if done.Status != "succeeded" || done.TotalMS != 130000 || *done.RuntimeEndMS != 9000 {
		t.Fatalf("done=%+v", done)
	}
	if len(done.Spans) != 3 {
		t.Fatalf("spans=%+v", done.Spans)
	}
	model := done.Spans[1]
	if model.Owner != "llm" || model.Name != "llm.video" || model.StartMS != 7000 || model.EndMS == nil || *model.EndMS != 125000 || model.Details.Label != "video-model" {
		t.Fatalf("model=%+v", model)
	}
	last := done.Spans[len(done.Spans)-1]
	if last.Owner != "csgclaw" || last.Name != "video.deliver" || last.StartMS != 125000 || last.EndMS == nil || *last.EndMS != 130000 {
		t.Fatalf("span=%+v", last)
	}
	json.Unmarshal(get("/api/v1/rooms/room/diagnostics?status=running"), &page)
	if len(page.Items) != 0 {
		t.Fatalf("completed job still running: %+v", page)
	}
	videos := (&Handler{im: svc}).diagnosticVideos("room")["turn"]
	enrichVideoDiagnostic(&snapshot, videos, true)
	if snapshot.TotalMS != 130000 {
		t.Fatalf("total=%v", snapshot.TotalMS)
	}
	original, _ := svc.Diagnostics().Get("room", snapshot.ID)
	if original.TotalMS != 9000 || original.Status != "succeeded" {
		t.Fatal("projection changed runtime lifecycle")
	}
	restored, err := im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatal(err)
	}
	routes = (&Handler{im: restored}).Routes()
	var reloaded diagnostics.Snapshot
	if err = json.Unmarshal(get("/api/v1/rooms/room/diagnostics/"+snapshot.ID), &reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.TotalMS != 130000 || reloaded.Status != "succeeded" {
		t.Fatalf("restored=%+v", reloaded)
	}

}

func TestVideoDiagnosticFailureAttribution(t *testing.T) {
	origin := time.Now().Add(-time.Minute)
	for _, test := range []struct {
		name        string
		completed   bool
		state       string
		modelStatus string
	}{
		{"model failure", false, "failed", "failed"},
		{"download failure", true, "failed", "completed"},
		{"attachment failure", true, "delivery_failed", "completed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := diagnostics.Snapshot{StartedAt: origin, AgentID: "agent", SourceID: "source", Status: "succeeded", TotalMS: 9000}
			task := contract.VideoGenerationTask{ID: "job", State: test.state, StartedAt: origin.Add(6 * time.Second).Format(time.RFC3339Nano), ModelStartedAt: origin.Add(7 * time.Second).Format(time.RFC3339Nano), EndedAt: origin.Add(20 * time.Second).Format(time.RFC3339Nano)}
			if test.completed {
				task.ModelCompletedAt = origin.Add(15 * time.Second).Format(time.RFC3339Nano)
			}
			enrichVideoDiagnostic(&record, []diagnosticVideo{{task: task, turn: channel.TurnContext{AgentID: "agent", SourceMessageID: "source"}}}, true)
			if record.Status != "failed" || record.TotalMS != 20000 {
				t.Fatalf("record=%+v", record)
			}
			model := record.Spans[1]
			if model.Owner != "llm" || model.Status != test.modelStatus || model.EndMS == nil {
				t.Fatalf("model=%+v", model)
			}
			if test.completed {
				delivery := record.Spans[2]
				if *model.EndMS != 15000 || delivery.Owner != "csgclaw" || delivery.Status != "failed" || *delivery.EndMS != 20000 {
					t.Fatalf("spans=%+v", record.Spans)
				}
			} else if *model.EndMS != 20000 {
				t.Fatalf("model=%+v", model)
			}
		})
	}
}

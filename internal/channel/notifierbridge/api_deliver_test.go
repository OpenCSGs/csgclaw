package notifierbridge

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"csgclaw/internal/apitypes"
)

type stubRoomLister struct {
	rooms map[string][]string
}

func (s stubRoomLister) RoomIDsForMember(memberID string) []string {
	if s.rooms == nil {
		return nil
	}
	return append([]string(nil), s.rooms[memberID]...)
}

func TestAPIDeliverPostsToEachRoom(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var posts []apitypes.CreateMessageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/messages" {
			http.NotFound(w, r)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req apitypes.CreateMessageRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		posts = append(posts, req)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"msg-1"}`))
	}))
	defer srv.Close()

	d := NewAPIDeliver(stubRoomLister{rooms: map[string][]string{
		"agent-1": {"room-a", "room-b"},
	}}, srv.URL, "")
	if err := d.DeliverNotifierFanout("agent-1", `{"notify":true}`); err != nil {
		t.Fatalf("DeliverNotifierFanout() error = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(posts) != 2 {
		t.Fatalf("posts = %d, want 2", len(posts))
	}
	for _, p := range posts {
		if p.SenderID != "agent-1" || !strings.Contains(p.Content, "notify") {
			t.Fatalf("post = %+v", p)
		}
	}
}

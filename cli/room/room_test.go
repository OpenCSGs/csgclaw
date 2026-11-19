package room

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"csgclaw/cli/command"
	"csgclaw/internal/apitypes"
)

func TestCreateRoomModes(t *testing.T) {
	for _, tc := range []struct {
		name, channel, wantError string
		flags                    []string
		wantMembers              []string
		wantType                 apitypes.RoomType
	}{
		{name: "default moderated", channel: "csgclaw", wantType: apitypes.RoomTypeOnDemand},
		{name: "empty local channel", wantType: apitypes.RoomTypeOnDemand},
		{name: "normalized local channel", channel: " CSGClaw ", wantType: apitypes.RoomTypeOnDemand},
		{name: "empty moderated", channel: "csgclaw", flags: []string{"--type", ""}, wantType: apitypes.RoomTypeOnDemand},
		{name: "moderated", channel: "csgclaw", flags: []string{"--type", "on_demand"}, wantType: apitypes.RoomTypeOnDemand},
		{name: "roundtable", channel: "csgclaw", flags: []string{"--type", "autonomous"}, wantType: apitypes.RoomTypeAutonomous},
		{name: "one invited agent remains a group", channel: "csgclaw", flags: []string{"--type", "autonomous", "--member-ids", "pt-dev"}, wantMembers: []string{"pt-dev"}, wantType: apitypes.RoomTypeAutonomous},
		{name: "reject free", channel: "csgclaw", flags: []string{"--type", "free"}, wantError: "free rooms can no longer be created"},
		{name: "reject padded free", channel: "csgclaw", flags: []string{"--type", " free "}, wantError: "free rooms can no longer be created"},
		{name: "reject invalid mode", channel: "csgclaw", flags: []string{"--type", "unknown"}, wantError: "type must be one of"},
		{name: "feishu native group", channel: "feishu", wantType: apitypes.RoomTypeFree},
		{name: "reject feishu collaboration", channel: "feishu", flags: []string{"--type", "autonomous"}, wantError: "collaboration rooms currently support csgclaw only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			routeChannel := "csgclaw"
			if tc.channel == "feishu" {
				routeChannel = "feishu"
			}
			requests := make(chan apitypes.CreateRoomRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/channels/"+routeChannel+"/rooms" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				var payload apitypes.CreateRoomRequest
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decode request: %v", err)
				}
				requests <- payload
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(apitypes.Room{ID: "created", Title: payload.Title, Type: payload.Type})
			}))
			defer server.Close()
			run := &command.Context{Program: "csgclaw-cli", Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HTTPClient: server.Client()}
			args := []string{"create", "--channel", tc.channel, "--title", "Build", "--creator-id", "admin", "--member-ids", "pt-dev, pt-qa"}
			err := NewCmd().Run(context.Background(), run, append(args, tc.flags...), command.GlobalOptions{Endpoint: server.URL, Output: "json"})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				select {
				case <-requests:
					t.Fatal("rejected mode reached the room creation API")
				default:
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			select {
			case payload := <-requests:
				wantMembers := tc.wantMembers
				if wantMembers == nil {
					wantMembers = []string{"pt-dev", "pt-qa"}
				}
				if payload.IsDirect || payload.TaskSession || payload.Type != tc.wantType || payload.Title != "Build" || payload.CreatorID != "admin" || !reflect.DeepEqual(payload.MemberIDs, wantMembers) {
					t.Fatalf("unexpected creation payload: %+v", payload)
				}
			default:
				t.Fatal("room creation did not reach the API")
			}
		})
	}
}

package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/cli/command"
	roomcmd "csgclaw/cli/room"
	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/im"
	"csgclaw/internal/roomtask"
	"csgclaw/internal/taskcore"
)

// Exercise the public upload, task context, CLI discovery and download surfaces
// together, with a DOCX clause that is not present in the assignment text.
func TestRoomAttachmentUploadDispatchCLIDownload(t *testing.T) {
	messages, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-dev", Name: "Dev", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "Contract", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeOnDemand})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: messages, serverAccessToken: "test-secret"}
	h.SetRoomTaskCore(taskcore.NewService())
	server := httptest.NewServer(h.Routes())
	defer server.Close()
	var document bytes.Buffer
	z := zip.NewWriter(&document)
	entry, err := z.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	clause := "验收后四十七个工作日，标识 RAVEN-6842"
	if _, err = io.WriteString(entry, "<document><text>"+clause+"</text></document>"); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	body, contentType := multipartMessageBodyForTest(t, map[string]any{"room_id": room.ID, "sender_id": "admin", "content": "Review the attached contract"}, "files", "合同.DOCX", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", document.Bytes())
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/messages", body)
	req.Header.Set("Content-Type", contentType)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var source im.Message
	if err = json.NewDecoder(resp.Body).Decode(&source); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated || len(source.Attachments) != 1 {
		t.Fatalf("upload: %d %+v", resp.StatusCode, source)
	}
	root, err := h.roomTaskSvc.Create(room.ID, source.ID, "admin", "Review contract", "Read clause nine")
	if err != nil {
		t.Fatal(err)
	}
	if err = h.roomTaskSvc.Plan(room.ID, root.ID, "Review", []roomtask.PlanItem{{IDRef: "dev", Title: "Read clause", Body: "Quote clause nine", AssignedTo: "pt-dev"}}, true); err != nil {
		t.Fatal(err)
	}
	var childID string
	for _, task := range h.roomTaskSvc.List(room.ID) {
		if task.ParentID == root.ID {
			childID = task.ID
		}
	}
	if err = h.roomTaskSvc.Dispatch(room.ID, childID, ""); err != nil {
		t.Fatal(err)
	}
	facts, err := h.roomExecutionContext(roomtask.TurnContextRequest{RoomID: room.ID, ParticipantID: "pt-dev", TaskID: childID, SourceID: "dispatch"})
	if err != nil || !strings.Contains(facts.TurnJSON, `"request_source_message_id":"`+source.ID+`"`) {
		t.Fatalf("source context: %+v %v", facts, err)
	}
	t.Setenv("CSGCLAW_CALLER_AGENT_ID", "agent-dev")
	var output bytes.Buffer
	run := &command.Context{Program: "csgclaw-cli", Stdout: &output, Stderr: io.Discard, HTTPClient: server.Client()}
	globals := command.GlobalOptions{Endpoint: server.URL, Token: "test-secret", Output: "json"}
	if err = roomcmd.NewCmd().Run(context.Background(), run, []string{"attachments", "list", "--room-id", room.ID, "--message-id", source.ID, "--query", "docx"}, globals); err != nil {
		t.Fatal(err)
	}
	var listed apitypes.RoomAttachmentList
	if err = json.Unmarshal(output.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Total != 1 || listed.Items[0].ID != source.Attachments[0].ID || listed.Items[0].SenderID != source.SenderID {
		t.Fatalf("listing: %+v", listed)
	}
	if strings.Contains(output.String(), "download_url") || strings.Contains(output.String(), "workspace_path") {
		t.Fatal("listing leaked delivery capability")
	}
	target := filepath.Join(t.TempDir(), "local-contract.docx")
	if err = roomcmd.NewCmd().Run(context.Background(), run, []string{"attachments", "download", "--room-id", room.ID, "--attachment-id", listed.Items[0].ID, "--output", target}, globals); err != nil {
		t.Fatal(err)
	}
	local, err := zip.OpenReader(target)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	contents, err := local.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer contents.Close()
	got, err := io.ReadAll(contents)
	if err != nil || !strings.Contains(string(got), clause) {
		t.Fatalf("clause missing: %q %v", got, err)
	}
	client := apiclient.New(server.URL, "test-secret", server.Client()).WithCallerAgentID("agent-dev")
	if _, err = client.DownloadRoomAttachment(context.Background(), room.ID, listed.Items[0].ID, target); err == nil {
		t.Fatal("overwrote existing output")
	}
	if _, err = messages.ClearRoomMessages(room.ID); err != nil {
		t.Fatal(err)
	}
	after, err := client.ListRoomAttachments(context.Background(), room.ID, apitypes.RoomAttachmentListOptions{})
	if err != nil || after.Total != 0 {
		t.Fatalf("cleared list: %+v %v", after, err)
	}
	if _, err = client.DownloadRoomAttachment(context.Background(), room.ID, listed.Items[0].ID, filepath.Join(t.TempDir(), "gone.docx")); err == nil {
		t.Fatal("downloaded cleared attachment")
	}
	if _, err = os.Stat(target); err != nil {
		t.Fatal("clearing room removed an explicit local download")
	}
}

func TestRoomAttachmentAuthorizationAndPagination(t *testing.T) {
	svc, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-dev", Name: "Dev", Role: "worker"}); err != nil {
		t.Fatal(err)
	}
	room, err := svc.CreateRoom(im.CreateRoomRequest{Title: "Files", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeFree})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.CreateRoom(im.CreateRoomRequest{Title: "Other", CreatorID: "admin", MemberIDs: []string{"dev"}, Type: apitypes.RoomTypeFree})
	if err != nil {
		t.Fatal(err)
	}
	source, err := svc.CreateMessage(im.CreateMessageRequest{RoomID: room.ID, SenderID: "admin", Attachments: []im.MessageAttachmentUpload{{Name: "Same.TXT", Data: []byte("one")}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.StartThread(im.StartThreadRequest{RoomID: room.ID, RootMessageID: source.ID}); err != nil {
		t.Fatal(err)
	}
	reply, err := svc.DeliverMessage(im.DeliverMessageRequest{RoomID: room.ID, SenderID: "dev", Content: "Published result", ThreadRootID: source.ID, Attachments: []im.MessageAttachmentUpload{{Name: "Same.TXT", Data: []byte("two")}}})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{im: svc, serverAccessToken: "secret"}
	call := func(path, token, caller string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if caller != "" {
			req.Header.Set("X-CSGClaw-Caller-Agent", caller)
		}
		out := httptest.NewRecorder()
		h.Routes().ServeHTTP(out, req)
		return out
	}
	path := "/api/v1/rooms/" + room.ID + "/attachments"
	for _, tc := range []struct {
		path, token, caller string
		status              int
	}{
		{path, "", "agent-dev", 401}, {path, "wrong", "agent-dev", 401}, {path, "secret", "agent-outsider", 403},
		{path, "secret", "agent-dev", 200}, {path, "secret", "", 200}, {path + "?limit=201", "secret", "agent-dev", 400},
		{path + "?from=-1", "secret", "agent-dev", 400}, {path + "?limit=0", "secret", "agent-dev", 400}, {path + "?from=no", "secret", "agent-dev", 400},
		{"/api/v1/rooms/" + other.ID + "/attachments/" + source.Attachments[0].ID, "secret", "agent-dev", 404},
	} {
		if out := call(tc.path, tc.token, tc.caller); out.Code != tc.status {
			t.Fatalf("%s: got %d want %d: %s", tc.path, out.Code, tc.status, out.Body.String())
		}
	}
	out := call(path+"?query=same.txt&limit=1", "secret", "agent-dev")
	var page apitypes.RoomAttachmentList
	if err = json.Unmarshal(out.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].MessageID != reply.ID {
		t.Fatalf("page: %+v", page)
	}
	out = call(path+"?from=1&limit=1", "secret", "agent-dev")
	if err = json.Unmarshal(out.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].MessageID != source.ID {
		t.Fatalf("second page: %+v", page)
	}
	if _, err = svc.RemoveRoomMembers(im.AddRoomMembersRequest{RoomID: room.ID, InviterID: "admin", UserIDs: []string{"dev"}}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, path + "/" + source.Attachments[0].ID} {
		if out := call(p, "secret", "agent-dev"); out.Code != 403 {
			t.Fatalf("removed member: %d", out.Code)
		}
	}
}

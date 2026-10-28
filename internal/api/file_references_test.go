package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/im"
	"csgclaw/internal/taskconfig"
)

func TestTaskWorkspaceListingPreviewAndReferenceUseLockedDirectory(t *testing.T) {
	h, _, alice, defaultRoot, _ := referenceTestHandler(t)
	taskRoot := t.TempDir()
	for root, content := range map[string]string{defaultRoot: "DEFAULT", taskRoot: "TASK-CONTENT"} {
		if err := os.WriteFile(filepath.Join(root, "same.md"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(taskRoot, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	room, err := h.im.CreateRoom(im.CreateRoomRequest{
		Title: "Configured task", CreatorID: "admin", MemberIDs: []string{alice}, IsDirect: true,
		Configuration: &taskconfig.Config{WorkspaceDir: taskRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	query := url.Values{"room_id": {room.ID}, "depth": {"1"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+"/workspace?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("docs")) {
		t.Fatalf("task listing: %d %s", rec.Code, rec.Body.String())
	}
	query.Set("path", "same.md")
	contentURL := "/api/v1/agents/" + alice + "/workspace/content?" + query.Encode()
	req = httptest.NewRequest(http.MethodGet, contentURL, nil)
	req.Header.Set("Range", "bytes=0-3")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "TASK" {
		t.Fatalf("task preview: %d %s", rec.Code, rec.Body.String())
	}
	code, message, body := postReference(t, h, room.ID, "task-reference", []FileReference{{Source: "workspace", AgentID: alice, Path: "same.md"}})
	if code != http.StatusCreated || len(message.Attachments) != 1 {
		t.Fatalf("task reference: %d %s", code, body)
	}
	stored, err := h.im.RoomAttachmentFile(room.ID, message.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(stored.Path)
	if err != nil || string(data) != "TASK-CONTENT" {
		t.Fatalf("wrong task snapshot: %q %v", data, err)
	}
	for _, invalid := range []string{"../same.md", "escape/same.md"} {
		if invalid == "escape/same.md" {
			if err := os.Symlink(defaultRoot, filepath.Join(taskRoot, "escape")); err != nil {
				t.Fatal(err)
			}
		}
		query.Set("path", invalid)
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+"/workspace/content?"+query.Encode(), nil)
		rec = httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, req)
		if rec.Code == http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("DEFAULT")) {
			t.Fatalf("task directory escape: %d %s", rec.Code, rec.Body.String())
		}
	}
	other, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Other", CreatorID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.conversationWorkspaceRoot(alice, other.ID); err == nil {
		t.Fatal("nonmember accessed a task workspace")
	}
}

func TestCollaborationWorkspaceListingPreviewAndReferenceUseSharedDirectory(t *testing.T) {
	h, _, alice, defaultRoot, _ := referenceTestHandler(t)
	taskRoot := t.TempDir()
	for root, content := range map[string]string{defaultRoot: "DEFAULT", taskRoot: "TASK-CONTENT"} {
		if err := os.WriteFile(filepath.Join(root, "same.md"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(taskRoot, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	room, err := h.im.CreateRoom(im.CreateRoomRequest{
		Title: "Configured task", CreatorID: "admin", MemberIDs: []string{alice}, Type: "on_demand",
		Configuration: &taskconfig.Config{WorkspaceDir: taskRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	query := url.Values{"room_id": {room.ID}, "depth": {"1"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+"/workspace?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("docs")) {
		t.Fatalf("task listing: %d %s", rec.Code, rec.Body.String())
	}
	query.Set("path", "same.md")
	contentURL := "/api/v1/agents/" + alice + "/workspace/content?" + query.Encode()
	req = httptest.NewRequest(http.MethodGet, contentURL, nil)
	req.Header.Set("Range", "bytes=0-3")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "TASK" {
		t.Fatalf("task preview: %d %s", rec.Code, rec.Body.String())
	}
	code, message, body := postReference(t, h, room.ID, "task-reference", []FileReference{{Source: "workspace", AgentID: alice, Path: "same.md"}})
	if code != http.StatusCreated || len(message.Attachments) != 1 {
		t.Fatalf("task reference: %d %s", code, body)
	}
	stored, err := h.im.RoomAttachmentFile(room.ID, message.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(stored.Path)
	if err != nil || string(data) != "TASK-CONTENT" {
		t.Fatalf("wrong task snapshot: %q %v", data, err)
	}
	for _, invalid := range []string{"../same.md", "escape/same.md"} {
		if invalid == "escape/same.md" {
			if err := os.Symlink(defaultRoot, filepath.Join(taskRoot, "escape")); err != nil {
				t.Fatal(err)
			}
		}
		query.Set("path", invalid)
		req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+"/workspace/content?"+query.Encode(), nil)
		rec = httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, req)
		if rec.Code == http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("DEFAULT")) {
			t.Fatalf("task directory escape: %d %s", rec.Code, rec.Body.String())
		}
	}
	other, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Other", CreatorID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.conversationWorkspaceRoot(alice, other.ID); err == nil {
		t.Fatal("nonmember accessed a task workspace")
	}
}

func TestLegacyCollaborationFilesRemainAvailableAcrossRestart(t *testing.T) {
	h, _, alice, aliceRoot, bobRoot := referenceTestHandler(t)
	// No state path means creation does not provision a shared directory, as in legacy data.
	h.im = im.NewServiceFromBootstrap(h.im.Bootstrap())
	room, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Legacy work", CreatorID: "admin", MemberIDs: []string{alice}, Type: "on_demand"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := im.SaveBootstrap(path, h.im.Bootstrap()); err != nil {
		t.Fatal(err)
	}
	for root, content := range map[string]string{aliceRoot: "ALICE-EXISTING", bobRoot: "BOB-PRIVATE"} {
		if err := os.WriteFile(filepath.Join(root, "existing.md"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		h.im, err = im.NewServiceFromPath(path)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := h.im.Room(room.ID)
		if got.Configuration != nil {
			t.Fatalf("legacy execution received a new room directory: %+v", got.Configuration)
		}
		query := url.Values{"room_id": {room.ID}, "path": {"existing.md"}}
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+"/workspace/content?"+query.Encode(), nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "ALICE-EXISTING" {
			t.Fatalf("legacy file preview: %d %s", rec.Code, rec.Body.String())
		}
	}
	code, message, body := postReference(t, h, room.ID, "legacy-file-reference", []FileReference{{Source: "workspace", AgentID: alice, Path: "existing.md"}})
	if code != http.StatusCreated || len(message.Attachments) != 1 {
		t.Fatalf("legacy file reference: %d %s", code, body)
	}
	stored, err := h.im.RoomAttachmentFile(room.ID, message.Attachments[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(stored.Path)
	if err != nil || string(data) != "ALICE-EXISTING" {
		t.Fatalf("wrong legacy file snapshot: %q %v", data, err)
	}
}

func referenceTestHandler(t *testing.T) (*Handler, string, string, string, string) {
	t.Helper()
	svc := mustNewService(t)
	var ids, roots []string
	for _, name := range []string{"alice", "bob"} {
		item, err := svc.CreateRecord(context.Background(), agent.CreateRequest{Spec: agent.CreateAgentSpec{Name: name, Role: agent.RoleWorker, RuntimeKind: agent.RuntimeKindCodex, Image: "worker:test"}})
		if err != nil {
			t.Fatal(err)
		}
		root, err := svc.WorkspaceRoot(item.Name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
		roots = append(roots, root)
	}
	messages, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, _, err := messages.EnsureAgentUser(im.EnsureAgentUserRequest{ID: id, Name: id, Role: "worker"}); err != nil {
			t.Fatal(err)
		}
	}
	room, err := messages.CreateRoom(im.CreateRoomRequest{Title: "References", CreatorID: "admin", MemberIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	return &Handler{svc: svc, workspace: svc.Workspace(), im: messages}, room.ID, ids[0], roots[0], roots[1]
}

func postReference(t *testing.T, h *Handler, roomID, key string, refs []FileReference) (int, im.Message, string) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"room_id": roomID, "sender_id": "admin", "content": "Review @same.md", "client_message_id": key, "file_references": refs})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", bytes.NewReader(data))
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	var message im.Message
	_ = json.Unmarshal(rec.Body.Bytes(), &message)
	return rec.Code, message, rec.Body.String()
}

func TestReferencedWorkspaceFileSnapshotDeliveryAndRetry(t *testing.T) {
	h, room, alice, source, target := referenceTestHandler(t)
	path := filepath.Join(source, "same.md")
	if err := os.WriteFile(path, []byte("ALICE-CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "same.md"), []byte("BOB-CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	ref := FileReference{Source: "workspace", AgentID: alice, Path: "same.md"}
	code, message, body := postReference(t, h, room, "reference-send", []FileReference{ref, ref})
	if code != http.StatusCreated || len(message.Attachments) != 1 {
		t.Fatalf("send: %d %s", code, body)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	code, retry, body := postReference(t, h, room, "reference-send", []FileReference{ref})
	if code != http.StatusCreated || retry.ID != message.ID {
		t.Fatalf("retry reopened source: %d %s", code, body)
	}
	attachment, err := h.im.MaterializeAttachment(message.Attachments[0].ID, target, "referenced-inputs")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(attachment.WorkspacePath)))
	if err != nil || string(data) != "ALICE-CONTENT" {
		t.Fatalf("recipient read wrong file: %q %v", data, err)
	}
	code, _, _ = postReference(t, h, room, "missing-source", []FileReference{ref})
	if code != http.StatusBadRequest {
		t.Fatalf("missing file: %d", code)
	}
	list, err := h.im.ListMessages(room)
	if err != nil || len(list) != 2 {
		t.Fatalf("partial/duplicate messages: %+v %v", list, err)
	}
}

func TestReferenceAuthorizationMultipartAndQuotas(t *testing.T) {
	h, room, alice, source, _ := referenceTestHandler(t)
	if err := os.WriteFile(filepath.Join(source, "same.md"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	ref := FileReference{Source: "workspace", AgentID: alice, Path: "same.md"}
	if _, release, err := h.resolveFileReferences(room, "another-agent", []FileReference{ref}); err == nil {
		release()
		t.Fatal("scoped runtime read another Agent workspace")
	}
	body, contentType := multipartMessageBodyForTest(t, map[string]any{"room_id": room, "sender_id": "admin", "content": "Both sources", "file_references": []FileReference{ref}}, "files", "upload.txt", "text/plain", []byte("upload"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	var message im.Message
	_ = json.Unmarshal(rec.Body.Bytes(), &message)
	if rec.Code != http.StatusCreated || len(message.Attachments) != 2 {
		t.Fatalf("multipart references: %d %s", rec.Code, rec.Body.String())
	}
	other, err := h.im.CreateRoom(im.CreateRoomRequest{Title: "Other", CreatorID: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := postReference(t, h, other.ID, "foreign-workspace", []FileReference{ref}); code != http.StatusBadRequest {
		t.Fatalf("foreign workspace accepted: %d", code)
	}
	if code, _, _ := postReference(t, h, other.ID, "foreign-attachment", []FileReference{{Source: "attachment", AttachmentID: message.Attachments[0].ID}}); code != http.StatusBadRequest {
		t.Fatalf("foreign attachment accepted: %d", code)
	}
	refs := make([]FileReference, 11)
	for i := range refs {
		name := string(rune('a'+i)) + ".txt"
		_ = os.WriteFile(filepath.Join(source, name), []byte("x"), 0600)
		refs[i] = FileReference{Source: "workspace", AgentID: alice, Path: name}
	}
	if code, _, _ := postReference(t, h, room, "quota", refs); code != http.StatusBadRequest {
		t.Fatalf("quota accepted: %d", code)
	}
	for _, path := range []string{"../outside", "/etc/passwd"} {
		if code, _, _ := postReference(t, h, room, path, []FileReference{{Source: "workspace", AgentID: alice, Path: path}}); code != http.StatusBadRequest {
			t.Fatalf("path accepted: %q %d", path, code)
		}
	}
}

func TestWorkspaceContentRangeAndDirectoryListing(t *testing.T) {
	h, _, alice, root, _ := referenceTestHandler(t)
	_ = os.Mkdir(filepath.Join(root, "docs"), 0700)
	_ = os.WriteFile(filepath.Join(root, "docs", "video.mp4"), []byte("0123456789"), 0600)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+"/workspace/content?path=docs/video.mp4", nil)
	req.Header.Set("Range", "bytes=3-6")
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "3456" || rec.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("range: %d %s %+v", rec.Code, rec.Body.String(), rec.Header())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+"/workspace?depth=1", nil)
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("video.mp4")) {
		t.Fatalf("listing recursed: %d %s", rec.Code, rec.Body.String())
	}
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "secret"), []byte("private"), 0600)
	_ = os.Symlink(outside, filepath.Join(root, "escape"))
	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+"/workspace/content?path=escape/secret", nil)
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || bytes.Contains(rec.Body.Bytes(), []byte("private")) {
		t.Fatal("content escaped workspace")
	}
	// A large sparse media file is still served with a bounded Range response.
	large, err := os.Create(filepath.Join(root, "large.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if err := large.Truncate(im.MaxAttachmentFileBytes); err != nil {
		t.Fatal(err)
	}
	_ = large.Close()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+alice+"/workspace/content?path=large.mp4", nil)
	req.Header.Set("Range", "bytes=0-7")
	rec = httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	data, _ := io.ReadAll(rec.Result().Body)
	if rec.Code != http.StatusPartialContent || len(data) != 8 {
		t.Fatalf("large range: %d %d", rec.Code, len(data))
	}
}

func TestReferencedDirectorySnapshotDeliveryAndRetry(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	h, room, alice, source, target := referenceTestHandler(t)
	directory := filepath.Join(source, "docs")
	if err := os.MkdirAll(filepath.Join(directory, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "nested", "readme.md"), []byte("directory snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	ref := FileReference{Source: "workspace", AgentID: alice, Path: "docs"}
	code, message, body := postReference(t, h, room, "directory-send", []FileReference{ref, ref})
	if code != http.StatusCreated || len(message.Attachments) != 1 {
		t.Fatalf("send: %d %s", code, body)
	}
	if message.Attachments[0].Name != "docs.zip" {
		t.Fatal(message.Attachments)
	}
	delivered, err := h.im.MaterializeAttachment(message.Attachments[0].ID, target, "referenced-inputs")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(filepath.Join(target, filepath.FromSlash(delivered.WorkspacePath)))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	found := false
	for _, file := range archive.File {
		if file.Name == "docs/nested/readme.md" {
			r, _ := file.Open()
			data, _ := io.ReadAll(r)
			r.Close()
			found = string(data) == "directory snapshot"
		}
	}
	if !found {
		t.Fatal("recipient did not receive directory contents")
	}
	if err := os.Symlink(filepath.Join(directory, "nested", "readme.md"), filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}
	code, _, body = postReference(t, h, room, "unsafe-directory", []FileReference{ref})
	if code != http.StatusBadRequest {
		t.Fatalf("symlink directory accepted: %d %s", code, body)
	}
	remaining, err := os.ReadDir(temporary)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("snapshot resources leaked: %v %v", remaining, err)
	}
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	code, retry, body := postReference(t, h, room, "directory-send", []FileReference{ref})
	if code != http.StatusCreated || retry.ID != message.ID {
		t.Fatalf("retry: %d %s", code, body)
	}
	code, _, _ = postReference(t, h, room, "deleted-directory", []FileReference{ref})
	if code != http.StatusBadRequest {
		t.Fatalf("deleted directory accepted: %d", code)
	}
}

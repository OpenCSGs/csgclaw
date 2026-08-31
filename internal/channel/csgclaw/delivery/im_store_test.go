package delivery

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge/runtimebridge"
	"csgclaw/internal/im"
)

type fixedParticipantResolver struct {
	item apitypes.Participant
}

func (r fixedParticipantResolver) Get(_, id string) (apitypes.Participant, bool) {
	return r.item, id == r.item.ID
}

type fixedOutputFileSource struct {
	files agentengine.FileInterface
}

func (s fixedOutputFileSource) Conversations(string) agentengine.ConversationInterface {
	return fixedOutputFileConversation{files: s.files}
}

type fixedOutputFileConversation struct {
	files agentengine.FileInterface
}

func (c fixedOutputFileConversation) Files() agentengine.FileInterface { return c.files }
func (fixedOutputFileConversation) Run(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
	return agentengine.TurnResult{}
}
func (fixedOutputFileConversation) Cancel(context.Context, agentengine.ConversationKey, agentengine.TurnID) error {
	return nil
}
func (fixedOutputFileConversation) Reset(context.Context, agentengine.ConversationKey) error {
	return nil
}
func (fixedOutputFileConversation) Resolve(context.Context, agentengine.InteractionResolution) error {
	return nil
}

func TestTranscriptRendererPersistsGeneratedFilesWithFinalMessage(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "im", "state.json")
	imService, err := im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatalf("NewServiceFromPath() error = %v", err)
	}
	if _, _, err := imService.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-worker", Name: "worker", Role: "worker"}); err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}
	room, err := imService.CreateRoom(im.CreateRoomRequest{
		Title: "Files", CreatorID: "user-admin", MemberIDs: []string{"user-worker"},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	fileStore := agentengine.NewFileStore()
	files := fileStore.Scope("agent-worker")
	markdown := []byte("# Generated report\n")
	created, err := files.Create(context.Background(), agentengine.FileCreateRequest{
		Name: "report.md", MIMEType: "text/markdown", SizeBytes: int64(len(markdown)),
	}, bytes.NewReader(markdown))
	if err != nil {
		t.Fatalf("Create(output file) error = %v", err)
	}
	store, err := NewIMTranscriptStore(imService, fixedParticipantResolver{item: apitypes.Participant{
		ID: "pt-worker", ChannelUserRef: "user-worker",
	}}, fixedOutputFileSource{files: files})
	if err != nil {
		t.Fatalf("NewIMTranscriptStore() error = %v", err)
	}
	renderer := NewTranscriptRenderer(store)
	turn := channel.TurnContext{
		AgentID: "agent-worker", ParticipantID: "pt-worker", RoomID: room.ID, Locale: "en",
		SourceMessageID: "message-1", ConversationKey: "conversation-1", TurnID: "turn-files",
	}
	missing := created
	missing.ID = "file-missing"
	missing.Name = "missing.pdf"
	if err := renderer.Complete(context.Background(), turn, agentengine.TurnResult{
		Status: agentengine.TurnSucceeded,
		Output: "The report is ready.",
		Files:  []agentengine.OutputFile{created, missing},
	}); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	persistedRoom, ok := imService.Room(room.ID)
	if !ok {
		t.Fatalf("Room() = false, want persisted room")
	}
	var message im.Message
	for _, candidate := range persistedRoom.Messages {
		if candidate.ID == "turn-files-final" {
			message = candidate
			break
		}
	}
	if message.ID != "turn-files-final" || !strings.Contains(message.Content, "The report is ready.") ||
		!strings.Contains(message.Content, "missing.pdf") {
		t.Fatalf("final message = %+v, want text and partial-delivery warning", message)
	}
	if len(message.Attachments) != 1 {
		t.Fatalf("attachments = %+v, want one generated file", message.Attachments)
	}
	attachment := message.Attachments[0]
	if attachment.Name != "report.md" || attachment.MediaType != "text/markdown" || attachment.SHA256 != created.SHA256 {
		t.Fatalf("attachment = %+v, want generated file metadata", attachment)
	}

	reloaded, err := im.NewServiceFromPath(statePath)
	if err != nil {
		t.Fatalf("reload IM service: %v", err)
	}
	file, err := reloaded.AttachmentFile(attachment.ID)
	if err != nil {
		t.Fatalf("AttachmentFile() after restart error = %v", err)
	}
	content, err := os.ReadFile(file.Path)
	if err != nil || !bytes.Equal(content, markdown) {
		t.Fatalf("persisted attachment content = %q, error = %v", content, err)
	}

	root, err := imService.CreateMessage(im.CreateMessageRequest{
		RoomID: room.ID, SenderID: "user-admin", Content: "root",
	})
	if err != nil {
		t.Fatalf("CreateMessage(root) error = %v", err)
	}
	threadTurn := turn
	threadTurn.TurnID = "turn-file-only"
	threadTurn.ThreadRootID = root.ID
	if err := store.DeliverFinalMessage(context.Background(), threadTurn, "", []agentengine.OutputFile{created}); err != nil {
		t.Fatalf("DeliverFinalMessage(file-only thread reply) error = %v", err)
	}
	allMessages, err := imService.ListMessagesWithOptions(room.ID, im.ListMessagesOptions{IncludeThreadReplies: true})
	if err != nil {
		t.Fatalf("ListMessagesWithOptions() error = %v", err)
	}
	var threadReply im.Message
	for _, candidate := range allMessages {
		if candidate.ID == "turn-file-only-final" {
			threadReply = candidate
			break
		}
	}
	if threadReply.Content != "" || len(threadReply.Attachments) != 1 || threadReply.RelatesTo == nil ||
		threadReply.RelatesTo.EventID != root.ID {
		t.Fatalf("file-only thread reply = %+v, messages = %+v", threadReply, allMessages)
	}
}

func TestOutputFileDeliveryTextLocalizesPartialFailures(t *testing.T) {
	if got := outputFileDeliveryText("done", []string{"report.pdf"}, "en"); got != "done\n\nCould not deliver generated file(s): \"report.pdf\"." {
		t.Fatalf("English warning = %q", got)
	}
	if got := outputFileDeliveryText("", []string{"报告.pdf"}, "zh-CN"); got != "无法发送生成的文件：\"报告.pdf\"。" {
		t.Fatalf("Chinese warning = %q", got)
	}
}

func TestIMTranscriptStoreWritesFinalAndUpdatesToolActivity(t *testing.T) {
	imService := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "user-admin",
		Users: []im.User{
			{ID: "user-admin", Name: "admin"},
			{ID: "user-worker", Name: "worker"},
		},
		Rooms: []im.Room{{
			ID: "room-1", IsDirect: true, Members: []string{"user-admin", "user-worker"},
		}},
	})
	store, err := NewIMTranscriptStore(imService, fixedParticipantResolver{item: apitypes.Participant{
		ID: "pt-worker", ChannelUserRef: "user-worker",
	}}, nil)
	if err != nil {
		t.Fatalf("NewIMTranscriptStore() error = %v", err)
	}
	turn := channel.TurnContext{
		ParticipantID: "pt-worker", RoomID: "room-1", SourceMessageID: "message-1",
		ConversationKey: "conversation-1", TurnID: "turn-1",
	}
	for _, status := range []string{"running", "completed"} {
		if err := store.DeliverActivity(context.Background(), turn, agentengine.TurnEvent{
			Kind: agentengine.TurnEventToolCallUpdate,
			Tool: &agentengine.ToolActivity{ID: "tool-1", Kind: "exec_command", Status: status},
		}); err != nil {
			t.Fatalf("DeliverActivity(%s) error = %v", status, err)
		}
	}
	if err := store.DeliverMessage(context.Background(), turn, "finished"); err != nil {
		t.Fatalf("DeliverMessage() error = %v", err)
	}
	if err := store.DeliverMessage(context.Background(), turn, "finished again"); err != nil {
		t.Fatalf("DeliverMessage(replay) error = %v", err)
	}

	room, ok := imService.Room("room-1")
	if !ok || len(room.Messages) != 2 {
		t.Fatalf("room = %+v, want replaced tool activity and one final response", room)
	}
	if room.Messages[0].SenderID != "user-worker" || !strings.Contains(room.Messages[0].Content, "completed") {
		t.Fatalf("tool message = %+v", room.Messages[0])
	}
	if room.Messages[1].SenderID != "user-worker" || room.Messages[1].Content != "finished again" || room.Messages[1].ID != "turn-1-final" {
		t.Fatalf("final message = %+v", room.Messages[1])
	}
	for _, message := range room.Messages {
		assertChannelMetadata(t, message.Metadata)
	}
}

func TestIMTranscriptStorePreservesActivityAndRuntimeErrorMetadata(t *testing.T) {
	imService := im.NewServiceFromBootstrap(im.Bootstrap{
		CurrentUserID: "user-admin",
		Users: []im.User{
			{ID: "user-admin", Name: "admin"},
			{ID: "user-worker", Name: "worker"},
		},
		Rooms: []im.Room{{
			ID: "room-1", IsDirect: true, Members: []string{"user-admin", "user-worker"},
		}},
	})
	store, err := NewIMTranscriptStore(imService, fixedParticipantResolver{item: apitypes.Participant{
		ID: "pt-worker", ChannelUserRef: "user-worker",
	}}, nil)
	if err != nil {
		t.Fatalf("NewIMTranscriptStore() error = %v", err)
	}
	turn := channel.TurnContext{
		ParticipantID: "pt-worker", RoomID: "room-1", SourceMessageID: "message-1",
		ConversationKey: "conversation-1", TurnID: "turn-1",
	}
	activityPayload := map[string]any{"type": runtimebridge.AgentActivityType}
	if err := store.DeliverRenderedActivity(context.Background(), turn, ActivityDelivery{
		MessageID: "question-1",
		Text:      "question",
		Metadata: map[string]any{
			runtimebridge.CSGClawMetadataKey: map[string]any{
				runtimebridge.AgentActivityMetaKey: activityPayload,
			},
		},
	}); err != nil {
		t.Fatalf("DeliverRenderedActivity() error = %v", err)
	}

	failureTurn := turn
	failureTurn.TurnID = "turn-2"
	if err := store.DeliverFailure(context.Background(), failureTurn, "unexpected status 429"); err != nil {
		t.Fatalf("DeliverFailure() error = %v", err)
	}

	room, ok := imService.Room("room-1")
	if !ok || len(room.Messages) != 2 {
		t.Fatalf("room = %+v, want question and failure", room)
	}
	assertChannelMetadata(t, room.Messages[0].Metadata)
	questionMetadata, _ := room.Messages[0].Metadata[runtimebridge.CSGClawMetadataKey].(map[string]any)
	if questionMetadata[runtimebridge.AgentActivityMetaKey] == nil {
		t.Fatalf("question metadata = %#v, want preserved agent activity", room.Messages[0].Metadata)
	}
	assertChannelMetadata(t, room.Messages[1].Metadata)
	failureMetadata, _ := room.Messages[1].Metadata[runtimebridge.CSGClawMetadataKey].(map[string]any)
	if failureMetadata[runtimebridge.RuntimeErrorMetaKey] != true || failureMetadata["error_code"] != "rate_limit_exceeded" {
		t.Fatalf("failure metadata = %#v, want runtime error fields", failureMetadata)
	}
}

func assertChannelMetadata(t *testing.T, metadata map[string]any) map[string]any {
	t.Helper()
	namespace, ok := metadata[channelMetadataKey].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v, want channel namespace", metadata)
	}
	if len(namespace) != 2 || namespace["type"] != string(channel.ChannelCSGClaw) || namespace["version"] != channelMetadataVersion {
		t.Fatalf("channel metadata = %#v, want only CSGClaw and version %d", namespace, channelMetadataVersion)
	}
	return namespace
}

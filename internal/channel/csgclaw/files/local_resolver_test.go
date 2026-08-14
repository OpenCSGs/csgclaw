package files

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/channelbridge"
	"csgclaw/internal/im"
)

type staticWorkspaceResolver struct {
	root string
}

func (r staticWorkspaceResolver) WorkspaceRootByID(string) (string, error) {
	return r.root, nil
}

type staticEngine struct {
	files agentengine.FileInterface
}

func (e staticEngine) Agents() agentengine.AgentInterface {
	return nil
}

func (e staticEngine) Conversations(string) agentengine.ConversationInterface {
	return staticConversation{files: e.files}
}

type staticConversation struct {
	files agentengine.FileInterface
}

func (c staticConversation) Files() agentengine.FileInterface {
	return c.files
}

func (staticConversation) Run(context.Context, agentengine.TurnRequest, agentengine.EventSink) agentengine.TurnResult {
	return agentengine.TurnResult{}
}

func (staticConversation) Cancel(context.Context, agentengine.ConversationKey, agentengine.TurnID) error {
	return nil
}

func (staticConversation) Reset(context.Context, agentengine.ConversationKey) error {
	return nil
}

func (staticConversation) Resolve(context.Context, agentengine.InteractionResolution) error {
	return nil
}

func TestLocalResolverStoresManagedAttachmentInAgentEngine(t *testing.T) {
	service, err := im.NewServiceFromPath(filepath.Join(t.TempDir(), "im", "state.json"))
	if err != nil {
		t.Fatalf("NewServiceFromPath() error = %v", err)
	}
	if _, _, err := service.EnsureAgentUser(im.EnsureAgentUserRequest{ID: "agent-worker", Name: "worker", Role: "worker"}); err != nil {
		t.Fatalf("EnsureAgentUser() error = %v", err)
	}
	room, err := service.CreateRoom(im.CreateRoomRequest{
		Title: "Ops", CreatorID: "user-admin", MemberIDs: []string{"user-worker"},
	})
	if err != nil {
		t.Fatalf("CreateRoom() error = %v", err)
	}
	payload := []byte("image fixture")
	message, err := service.CreateMessage(im.CreateMessageRequest{
		RoomID: room.ID, SenderID: "user-admin", Attachments: []im.MessageAttachmentUpload{{
			Name: "diagram.png", MediaType: "image/png", Data: payload,
		}},
	})
	if err != nil {
		t.Fatalf("CreateMessage() error = %v", err)
	}
	if len(message.Attachments) != 1 {
		t.Fatalf("attachments = %+v, want one attachment", message.Attachments)
	}

	workspaceRoot := t.TempDir()
	fileStore := agentengine.NewFileStore().Scope("agent-worker")
	resolver, err := NewLocalResolver(service, staticWorkspaceResolver{root: workspaceRoot}, staticEngine{files: fileStore})
	if err != nil {
		t.Fatalf("NewLocalResolver() error = %v", err)
	}
	attachment := message.Attachments[0]
	input, release, err := resolver.Resolve(
		context.Background(),
		channel.Binding{AgentID: "agent-worker"},
		channelbridge.BotEvent{RoomID: room.ID, MessageID: message.ID, Attachments: message.Attachments},
		attachment,
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if input.ID == "" {
		t.Fatal("InputFile.ID is empty")
	}
	download, err := fileStore.Get(context.Background(), input.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	got, readErr := io.ReadAll(download.Content)
	closeErr := download.Content.Close()
	if readErr != nil || closeErr != nil || string(got) != string(payload) {
		t.Fatalf("stored content = %q, readErr=%v, closeErr=%v, want %q", string(got), readErr, closeErr, string(payload))
	}
	if release == nil {
		t.Fatal("release = nil, want managed attachment cleanup")
	}
	release()
	if _, err := fileStore.Get(context.Background(), input.ID); agentengine.ErrorCodeOf(err) != agentengine.ErrorFileNotFound {
		t.Fatalf("Get() after release error = %v, want file_not_found", err)
	}
	managedFiles, err := filepath.Glob(filepath.Join(workspaceRoot, ".csgclaw", "attachments", "*", "*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range managedFiles {
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().IsRegular() {
			t.Fatalf("managed source still exists after release: %s", path)
		}
	}
}

func TestResolveManagedAttachmentSourcePathRejectsOutsideWorkspace(t *testing.T) {
	workspaceRoot := t.TempDir()
	managedRoot := filepath.Join(workspaceRoot, ".csgclaw", "attachments")
	if err := os.MkdirAll(managedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveManagedAttachmentSourcePath(workspaceRoot, outsidePath); err == nil {
		t.Fatal("resolveManagedAttachmentSourcePath() error = nil, want outside path rejected")
	}
}

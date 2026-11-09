package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
)

func TestStructuredQuestionPreservesReadableOutput(t *testing.T) {
	adapter := &ConversationAdapter{}
	var events []contract.TurnEvent
	var output conversationOutput
	var files []*contract.OutputFile
	text := "## 交互式输出演示 - 第 1/3 步\n\n请选择工作流分支。"
	result := adapter.handleEvent(context.Background(), contract.TurnRequest{}, contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error {
		events = append(events, event)
		return nil
	}), activity.RuntimeEvent{
		Kind: activity.RuntimeEventStructuredOutput,
		Text: text,
		Payload: activity.StructuredOutputArtifact{
			RequestUserInput: &activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{{ID: "demo_kind", Header: "演示类型", Question: "请选择工作流分支。"}}},
			ResourceLinks:    []activity.ResourceLink{{Type: "resource_link", Name: "docs", URI: "https://example.com/docs"}},
		},
	}, &output, &files)
	if result != nil || len(events) != 2 {
		t.Fatalf("result = %+v, events = %+v", result, events)
	}
	if events[0].Output.Kind != contract.OutputItemRequestUserInput || events[0].Text != text {
		t.Fatalf("question event = %+v, want readable output preserved", events[0])
	}
	if events[1].Output.Kind != contract.OutputItemResourceLink || events[1].Text != "" {
		t.Fatalf("link event = %+v, want link without duplicate text", events[1])
	}
}

type inputLifecycleBackend struct {
	ConversationBackend
	workspace string
}

func (b inputLifecycleBackend) RuntimeDir(string) (string, error) {
	return filepath.Dir(b.workspace), nil
}

func (b inputLifecycleBackend) WorkspaceDir(string) (string, error) { return b.workspace, nil }

func TestInputFileExplainsTemporaryPathAndPreservesExplicitSavedCopy(t *testing.T) {
	workspace := t.TempDir()
	adapter := &ConversationAdapter{runtime: inputLifecycleBackend{workspace: workspace}}
	payload := []byte("uploaded video")
	file, err := contract.NewOutputFile(context.Background(), contract.OutputFileMetadata{Name: "video.mkv", MediaType: "video/matroska", SizeBytes: int64(len(payload))}, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Cleanup()
	blocks, cleanup, inputErr := adapter.prepareInput(context.Background(), "upload-turn", []contract.InputPart{{Kind: contract.InputPartFile, File: &contract.InputFile{ID: file.ID, Resolved: file}}})
	if inputErr != nil {
		t.Fatal(inputErr)
	}
	defer cleanup()
	text := blocks[0].Text.Text
	for _, want := range []string{"temporarily available", "deleted when the turn ends", "copy it", "verify", "list/download"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q", want)
		}
	}
	inputs, err := filepath.Glob(filepath.Join(workspace, ".csgclaw", "engine-inputs", "*", "*"))
	if err != nil || len(inputs) != 1 {
		t.Fatalf("inputs=%v %v", inputs, err)
	}
	saved := filepath.Join(workspace, "saved-video.mkv")
	content, err := os.ReadFile(inputs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(saved, content, 0600); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := os.Stat(inputs[0]); !os.IsNotExist(err) {
		t.Fatal("temporary input survived the turn")
	}
	content, err = os.ReadFile(saved)
	if err != nil || !bytes.Equal(content, payload) {
		t.Fatal("explicit saved copy lost")
	}
}

func TestConversationAdapterPreservesCommentaryOutsideFinalOutput(t *testing.T) {
	adapter := &ConversationAdapter{}
	var output conversationOutput
	var files []*contract.OutputFile
	var events []contract.TurnEvent
	sink := contract.EventSinkFunc(func(_ context.Context, event contract.TurnEvent) error { events = append(events, event); return nil })
	for _, event := range []activity.RuntimeEvent{
		{Kind: activity.RuntimeEventTextDelta, MessageID: "intro", Text: "Checking.", Payload: map[string]any{"phase": "commentary"}},
		{Kind: activity.RuntimeEventTextDelta, MessageID: "answer", Text: "Answer.", Payload: map[string]any{"phase": "final_answer"}},
	} {
		if result := adapter.handleEvent(context.Background(), contract.TurnRequest{}, sink, event, &output, &files); result != nil {
			t.Fatal(result)
		}
	}
	if len(events) != 2 || events[0].Phase != "commentary" || events[0].ItemID != "intro" || output.String() != "Answer." {
		t.Fatalf("events=%+v output=%s", events, output.String())
	}
}

func TestCompletedMessageSnapshotReplacesOnlyItsOwnOutput(t *testing.T) {
	var text conversationText
	text.apply("intro", "commentary", "Checking", false)
	text.apply("a", "final_answer", "First. ", false)
	text.apply("b", "final_answer", "Draft", false)
	text.apply("b", "final_answer", "Corrected.", true)
	if text.String() != "First. Corrected." {
		t.Fatal(text.String())
	}
}

func TestUnidentifiedCommentaryNeverEntersFinalOutput(t *testing.T) {
	var text conversationText
	text.apply("", "commentary", "Checking.", false)
	text.apply("", "final_answer", "Answer.", false)
	if text.String() != "Answer." {
		t.Fatal(text.String())
	}
}

func TestWorkspaceAttachmentPathsSurviveCodexTurns(t *testing.T) {
	workspace := t.TempDir()
	adapter := &ConversationAdapter{runtime: inputLifecycleBackend{workspace: workspace}}
	relative := ".csgclaw/attachments/sessions/session/batch/照片.png"
	path := filepath.Join(workspace, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	part := contract.InputPart{Kind: contract.InputPartWorkspaceFile, WorkspaceFile: &contract.WorkspaceFile{Path: relative, Name: "照片.png", MIMEType: "image/png"}}
	for _, content := range []string{"original", "edited"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		blocks, cleanup, turnErr := adapter.prepareInput(context.Background(), "turn", []contract.InputPart{part})
		if turnErr != nil {
			t.Fatal(turnErr)
		}
		defer cleanup()
		if len(blocks) != 2 || blocks[0].Text == nil || !strings.Contains(blocks[0].Text.Text, relative) || strings.Contains(blocks[0].Text.Text, workspace) || blocks[1].LocalImage == nil {
			t.Fatalf("workspace image blocks = %+v", blocks)
		}
		preview := blocks[1].LocalImage.Path
		if filepath.Ext(preview) != ".png" || pathWithinRuntimeDir(workspace, preview) {
			t.Fatalf("private preview = %s", preview)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		linked, err := os.Lstat(preview)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(info, linked) {
			t.Fatal("expected a same-filesystem hard link")
		}
		got, err := os.ReadFile(preview)
		if err != nil || string(got) != content {
			t.Fatalf("image=%q error=%v", got, err)
		}
		cleanup()
		if _, err := os.Stat(preview); !os.IsNotExist(err) {
			t.Fatalf("preview survived cleanup: %v", err)
		}
		got, err = os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("workspace image=%q error=%v", got, err)
		}
	}
	if err := os.Truncate(path, apitypes.MaxAttachmentFileBytes+1); err != nil {
		t.Fatal(err)
	}
	blocks, cleanup, turnErr := adapter.prepareInput(context.Background(), "oversized", []contract.InputPart{part})
	cleanup()
	if turnErr != nil || len(blocks) != 1 || blocks[0].Text == nil || !strings.Contains(blocks[0].Text.Text, relative) {
		t.Fatalf("oversized image blocks = %+v, error = %v", blocks, turnErr)
	}
	part.WorkspaceFile.MIMEType = "application/octet-stream"
	blocks, cleanup, turnErr = adapter.prepareInput(context.Background(), "later", []contract.InputPart{part})
	cleanup()
	if turnErr != nil || len(blocks) != 1 || !strings.Contains(blocks[0].Text.Text, relative) {
		t.Fatalf("file blocks=%+v error=%v", blocks, turnErr)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".csgclaw", "engine-inputs")); !os.IsNotExist(err) {
		t.Fatalf("temporary input directory: %v", err)
	}
}

func TestWorkspaceImagePreviewUsesOpenedFileAfterPathReplacement(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "photo.png")
	if err := os.WriteFile(source, []byte("uploaded"), 0600); err != nil {
		t.Fatal(err)
	}
	opened, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if err := os.Rename(source, filepath.Join(root, "moved")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("open workspace file cannot be renamed: %v", err)
		}
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	preview := filepath.Join(t.TempDir(), "preview.png")
	info, err := opened.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareWorkspaceImage(context.Background(), opened, info, source, preview); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(preview)
	if err != nil || string(got) != "uploaded" {
		t.Fatalf("preview=%q error=%v", got, err)
	}
}

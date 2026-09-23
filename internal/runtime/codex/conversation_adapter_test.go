package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine/contract"
)

func TestStructuredQuestionPreservesReadableOutput(t *testing.T) {
	adapter := &ConversationAdapter{}
	var events []contract.TurnEvent
	var output strings.Builder
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

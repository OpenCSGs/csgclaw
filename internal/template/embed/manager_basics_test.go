package templateembed

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

func TestManagerBasicsRoomCreationKeepsRequesterAsCreator(t *testing.T) {
	data, err := fs.ReadFile(FS(), path.Join(CodexManagerRoot, WorkspaceDirName, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read codex manager instructions: %v", err)
	}
	instructions := string(data)

	for _, want := range []string{
		"CSGClaw Codex Manager",
		"csgclaw-cli room create --title test-room --creator-id admin --member-ids manager,<worker-participant-id> --channel csgclaw",
		"Resolve worker participant IDs with `participant list` before using them.",
		"preserve the requester as `--creator-id`",
		"include `manager` plus the requested participants in `--member-ids`",
		"Do not use `manager` as the creator just because the manager runs the CLI command.",
		"a display name such as `dev` or `qa` is not necessarily a valid participant ID.",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("codex manager instructions missing room guidance %q", want)
		}
	}
	if strings.Contains(instructions, "skills/basics") {
		t.Fatalf("codex manager instructions still reference basics skill:\n%s", instructions)
	}
	if strings.Contains(instructions, "--creator-id manager") {
		t.Fatalf("codex manager instructions still teach manager as room creator:\n%s", instructions)
	}
	if strings.Contains(instructions, "--member-ids manager,dev") {
		t.Fatalf("codex manager instructions still teach sample dev as a literal participant ID:\n%s", instructions)
	}
}

func TestManagerInstructionsPreferAgentTasksForSingleWorkerDispatch(t *testing.T) {
	data, err := fs.ReadFile(FS(), path.Join(CodexManagerRoot, WorkspaceDirName, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read codex manager instructions: %v", err)
	}
	instructions := string(data)

	for _, want := range []string{
		"Single-worker task assignment second",
		"csgclaw-cli task create --agent-id <worker_agent_id>",
		"Do not create a room or send a manual assignment message for this path.",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("codex manager instructions missing task API dispatch guidance %q", want)
		}
	}
	if strings.Contains(instructions, "Dispatch means waking a worker with a real IM mention") {
		t.Fatalf("codex manager instructions still define dispatch as manual IM mention:\n%s", instructions)
	}
}

func TestWorkerInstructionsMentionDirectAgentTaskCLI(t *testing.T) {
	tests := []struct {
		name string
		root string
		file string
	}{
		{name: "codex", root: CodexWorkerRoot, file: "AGENTS.md"},
		{name: "picoclaw", root: PicoClawWorkerRoot, file: "AGENT.md"},
		{name: "openclaw", root: OpenClawWorkerRoot, file: "AGENTS.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := fs.ReadFile(FS(), path.Join(tt.root, WorkspaceDirName, tt.file))
			if err != nil {
				t.Fatalf("read worker instructions: %v", err)
			}
			instructions := string(data)
			for _, want := range []string{
				"csgclaw-cli task claim --task <task_id>",
				"csgclaw-cli task update --task <task_id>",
				"Do not use `team task` commands for direct agent tasks.",
			} {
				if !strings.Contains(instructions, want) {
					t.Fatalf("worker instructions missing direct agent task guidance %q", want)
				}
			}
		})
	}
}

func TestManagerFeishuSkillRoomCreationKeepsRequesterAsCreator(t *testing.T) {
	data, err := fs.ReadFile(FS(), path.Join(CodexManagerRoot, WorkspaceDirName, "skills/feishu/SKILL.md"))
	if err != nil {
		t.Fatalf("read codex manager feishu skill: %v", err)
	}
	skill := string(data)

	for _, want := range []string{
		"csgclaw-cli room create --title worker-group --creator-id admin --member-ids manager,<worker-participant-id> --channel feishu",
		"keep the human requester as `--creator-id`",
		"Include `manager` plus the requested worker participant IDs in `--member-ids`",
		"replace `<worker-participant-id>` with IDs from `participant list`",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("feishu skill missing requester creator guidance %q", want)
		}
	}
	if strings.Contains(skill, "--creator-id manager") {
		t.Fatalf("feishu skill still teaches manager as room creator:\n%s", skill)
	}
	if strings.Contains(skill, "--member-ids manager,dev") {
		t.Fatalf("feishu skill still teaches sample dev as a literal participant ID:\n%s", skill)
	}
}

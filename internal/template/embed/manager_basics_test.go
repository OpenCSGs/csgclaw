package templateembed

import (
	"errors"
	"io/fs"
	"path"
	"strings"
	"testing"
)

func TestManagerTemplateLeavesConversationPolicyToRuntime(t *testing.T) {
	data, err := fs.ReadFile(FS(), path.Join(CodexManagerRoot, InstructionsDirName, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"Conversation Mode Priority", "Casual Messages", "Explicit CSGClaw Administration", "task submit", "task plan", "agent-teams"} {
		if strings.Contains(string(data), unwanted) {
			t.Errorf("manager template still contains %q", unwanted)
		}
	}
}

func TestTemplatesDoNotBundleRetiredTeamSkill(t *testing.T) {
	for _, root := range []string{CodexManagerRoot, CodexWorkerRoot, DSHWorkerRoot} {
		if _, err := fs.Stat(FS(), path.Join(root, SkillsDirName, "agent-teams")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%s bundles the retired Team skill: %v", root, err)
		}
	}
}

func TestWorkerTemplatesLeaveTaskRoutingToRuntime(t *testing.T) {
	for _, root := range []string{CodexWorkerRoot, DSHWorkerRoot} {
		data, err := fs.ReadFile(FS(), path.Join(root, InstructionsDirName, "AGENTS.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, policy := range []string{"task claim --task", "task update --task", "team task claim --team", "team task update --team"} {
			if strings.Contains(string(data), policy) {
				t.Errorf("%s template still embeds %q", root, policy)
			}
		}
	}
}

func TestManagerFeishuSkillRoomCreationKeepsRequesterAsCreator(t *testing.T) {
	data, err := fs.ReadFile(FS(), path.Join(CodexManagerRoot, SkillsDirName, "feishu/SKILL.md"))
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

func TestManagerEmbedsInteractiveOutputDemo(t *testing.T) {
	skillRoot := path.Join(CodexManagerRoot, SkillsDirName, "csgclaw-interactive-output-demo")
	for _, file := range []string{
		"SKILL.md",
		"agents/openai.yaml",
		"scripts/emit_demo.py",
		"references/stage-2.md",
		"references/stage-3.md",
		"references/complete.md",
	} {
		if _, err := fs.ReadFile(FS(), path.Join(skillRoot, file)); err != nil {
			t.Fatalf("read embedded interactive output demo %s: %v", file, err)
		}
	}

	metadata, err := fs.ReadFile(FS(), path.Join(skillRoot, "agents/openai.yaml"))
	if err != nil {
		t.Fatalf("read embedded interactive output demo metadata: %v", err)
	}
	if !strings.Contains(string(metadata), "allow_implicit_invocation: false") {
		t.Fatalf("interactive output demo must remain explicit-only:\n%s", metadata)
	}
}

func TestManagerInstructionsEnforceStructuredOutputTurnBoundary(t *testing.T) {
	instructions, err := fs.ReadFile(FS(), path.Join(CodexManagerRoot, InstructionsDirName, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read embedded manager instructions: %v", err)
	}

	for _, want := range []string{
		"::csgclaw-output::request_user_input",
		"native Codex `request_user_input` is unavailable",
		"Never add a third leading colon",
		"final tool call of the current turn",
		"do not call another tool",
		"RequestUserInputResponse",
		"never from tool stdout produced in the current turn",
	} {
		if !strings.Contains(string(instructions), want) {
			t.Fatalf("manager instructions missing structured-output boundary %q", want)
		}
	}
}

func TestWorkerInstructionsExplainClickableQuestionOutput(t *testing.T) {
	instructions, err := fs.ReadFile(FS(), path.Join(CodexWorkerRoot, InstructionsDirName, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read embedded worker instructions: %v", err)
	}

	for _, want := range []string{
		"native Codex `request_user_input` is unavailable",
		"::csgclaw-output::request_user_input",
		"Never add a third leading colon",
		"end the turn immediately",
	} {
		if !strings.Contains(string(instructions), want) {
			t.Fatalf("worker instructions missing clickable question guidance %q", want)
		}
	}
}

func TestManagerScheduledTaskSkillIsEmbedded(t *testing.T) {
	for _, name := range []string{"SKILL.md", "agents/openai.yaml"} {
		if _, err := fs.ReadFile(FS(), path.Join(CodexManagerRoot, SkillsDirName, "scheduled-tasks", name)); err != nil {
			t.Fatal(err)
		}
	}
	instructions, err := fs.ReadFile(FS(), path.Join(CodexManagerRoot, InstructionsDirName, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(instructions), "skills/scheduled-tasks/SKILL.md") {
		t.Fatal("missing scheduling route")
	}
}

package codex

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"csgclaw/internal/activity"
)

func TestDecodeStructuredCommandOutputRoutesCodexRecordsAndPreservesStdout(t *testing.T) {
	t.Parallel()

	output := strings.Join([]string{
		"ordinary stdout before",
		`::csgclaw-output::resource_link {"type":"resource_link","name":"docs","title":"CSGClaw docs","uri":"https://opencsg.com/docs?q=结构化","description":"Guide, examples, and API.","mimeType":"text/html","size":42,"annotations":{"audience":["user"]},"_meta":{"source":"demo"},"icons":[{"src":"https://opencsg.com/icon.png"}]}`,
		`::csgclaw-output::request_user_input {"autoResolutionMs":240000,"questions":[{"id":"kind","header":"Demo kind","question":"What kind of demo should this be?","isOther":true,"isSecret":false,"options":[{"label":"Bug fix (Recommended)","description":"Repair, verify, and report."}]},{"id":"detail","header":"Details","question":"Add details with punctuation: commas, spaces, and 中文。","isOther":true,"isSecret":false,"options":null},{"id":"three","header":"Three","question":"Question three?","isOther":false,"isSecret":false,"options":[]},{"id":"four","header":"Four","question":"Question four?","isOther":false,"isSecret":false,"options":[]},{"id":"five","header":"Five","question":"Question five?","isOther":true,"isSecret":true,"options":null}]}`,
		`::csgclaw-output::resource_link {"type":"resource_link","name":"minimal","uri":"https://example.com/minimal"}`,
		"ordinary stdout after",
	}, "\n")

	cleaned, artifact, errs := decodeStructuredCommandOutput(output)
	if len(errs) != 0 {
		t.Fatalf("decode errors = %v", errs)
	}
	if cleaned != "ordinary stdout before\nordinary stdout after" {
		t.Fatalf("cleaned output = %q", cleaned)
	}
	if artifact.RequestUserInput == nil || len(artifact.RequestUserInput.Questions) != 5 {
		t.Fatalf("request = %+v, want five questions", artifact.RequestUserInput)
	}
	if artifact.RequestUserInput.AutoResolutionMS == nil || *artifact.RequestUserInput.AutoResolutionMS != 240_000 {
		t.Fatalf("autoResolutionMs = %v, want 240000", artifact.RequestUserInput.AutoResolutionMS)
	}
	if got := artifact.RequestUserInput.Questions[0].Options[0].Label; got != "Bug fix (Recommended)" {
		t.Fatalf("recommended label = %q", got)
	}
	if len(artifact.ResourceLinks) != 2 || artifact.ResourceLinks[0].MIMEType != "text/html" || artifact.ResourceLinks[0].Size == nil {
		t.Fatalf("resource links = %+v", artifact.ResourceLinks)
	}
}

func TestDecodeStructuredCommandOutputRejectsMalformedDuplicateAndUnsafeRecords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record string
	}{
		{name: "malformed json", record: `::csgclaw-output::request_user_input {not-json}`},
		{name: "duplicate question ids", record: `::csgclaw-output::request_user_input {"questions":[{"id":"same","header":"One","question":"One?","options":[]},{"id":"same","header":"Two","question":"Two?","options":[]}]}`},
		{name: "unsafe link", record: `::csgclaw-output::resource_link {"type":"resource_link","name":"bad","uri":"javascript:alert(1)"}`},
		{name: "wrong link type", record: `::csgclaw-output::resource_link {"type":"text","name":"bad","uri":"https://example.com"}`},
		{name: "oversized", record: structuredOutputPrefix + structuredOutputResourceLink + " " + strings.Repeat("x", maxStructuredOutputRecordBytes)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cleaned, artifact, errs := decodeStructuredCommandOutput("before\n" + test.record + "\nafter")
			if !structuredOutputArtifactEmpty(artifact) {
				t.Fatalf("artifact = %+v, want empty", artifact)
			}
			if len(errs) == 0 {
				t.Fatal("decode errors = nil, want rejection")
			}
			if !strings.Contains(cleaned, test.record) {
				t.Fatalf("cleaned output removed invalid record: %q", cleaned)
			}
		})
	}
}

func TestDecodeStructuredCommandOutputEnforcesLimitsAndDeduplicatesLinks(t *testing.T) {
	t.Parallel()

	questions := make([]string, maxStructuredOutputQuestions+1)
	for index := range questions {
		questions[index] = fmt.Sprintf(`{"id":"q-%d","header":"Q","question":"Q?","options":[]}`, index)
	}
	tooManyQuestions := `::csgclaw-output::request_user_input {"questions":[` + strings.Join(questions, ",") + `]}`
	_, artifact, errs := decodeStructuredCommandOutput(tooManyQuestions)
	if artifact.RequestUserInput != nil || len(errs) != 1 {
		t.Fatalf("too many questions artifact=%+v errors=%v", artifact, errs)
	}
	options := make([]string, maxStructuredOutputQuestionOptions+1)
	for index := range options {
		options[index] = fmt.Sprintf(`{"label":"option-%d","description":"description"}`, index)
	}
	tooManyOptions := `::csgclaw-output::request_user_input {"questions":[{"id":"q","header":"Q","question":"Q?","options":[` + strings.Join(options, ",") + `]}]}`
	_, artifact, errs = decodeStructuredCommandOutput(tooManyOptions)
	if artifact.RequestUserInput != nil || len(errs) != 1 {
		t.Fatalf("too many options artifact=%+v errors=%v", artifact, errs)
	}

	var records []string
	for index := 0; index < maxStructuredOutputResourceLinks+2; index++ {
		records = append(records, fmt.Sprintf(`::csgclaw-output::resource_link {"type":"resource_link","name":"link-%d","uri":"https://example.com/%d"}`, index, index))
	}
	records = append(records, records[0])
	cleaned, artifact, errs := decodeStructuredCommandOutput(strings.Join(records, "\n"))
	if cleaned != "" || len(artifact.ResourceLinks) != maxStructuredOutputResourceLinks {
		t.Fatalf("cleaned=%q links=%d", cleaned, len(artifact.ResourceLinks))
	}
	if len(errs) != 2 {
		t.Fatalf("errors = %v, want one per over-limit unique link", errs)
	}
}

func TestStructuredOutputOnlyPublishesForSuccessfulToolStatus(t *testing.T) {
	t.Parallel()

	record := `::csgclaw-output::resource_link {"type":"resource_link","name":"docs","uri":"https://example.com/docs"}`
	for _, status := range []string{"failed", "cancelled", "canceled", "superseded", "in_progress"} {
		if structuredOutputToolStatusSuccessful(status) {
			t.Fatalf("status %q accepted", status)
		}
	}
	for _, status := range []string{"completed", "success", "succeeded"} {
		if !structuredOutputToolStatusSuccessful(status) {
			t.Fatalf("status %q rejected", status)
		}
	}

	sink := &recordingSink{}
	manager := newAppServerManager(testAppServerManagerDepsWithSink(sink))
	if cleaned := manager.decodeAndPublishStructuredCommandOutput("rt-1", "session-1", "tool-failed", "failed", record, nil, nil); cleaned != record {
		t.Fatalf("failed cleaned output = %q, want untouched", cleaned)
	}
	if events := sink.snapshot(); len(events) != 0 {
		t.Fatalf("failed events = %+v, want none", events)
	}
	if cleaned := manager.decodeAndPublishStructuredCommandOutput("rt-1", "session-1", "tool-ok", "completed", record, nil, nil); cleaned != "" {
		t.Fatalf("completed cleaned output = %q", cleaned)
	}
	events := sink.snapshot()
	if len(events) != 1 || events[0].Kind != activity.RuntimeEventStructuredOutput || events[0].ToolCallID != "tool-ok" {
		t.Fatalf("completed events = %+v", events)
	}
}

func TestEmbeddedInteractiveOutputDemoExercisesEveryPositiveFeature(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("..", "..", "template", "embed", "manager", "codex", "workspace", "skills", "csgclaw-interactive-output-demo")
	command := exec.Command("python3", filepath.Join(dir, "scripts", "emit_demo.py"))
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run demo emitter: %v", err)
	}
	cleaned, artifact, decodeErrors := decodeStructuredCommandOutput(string(output))
	if len(decodeErrors) != 0 {
		t.Fatalf("decode demo emitter: %v", decodeErrors)
	}
	if strings.TrimSpace(cleaned) != "Interactive output demo controls emitted successfully." {
		t.Fatalf("ordinary stdout = %q", cleaned)
	}
	if len(artifact.ResourceLinks) != 2 {
		t.Fatalf("resource links = %d, want full and minimal variants", len(artifact.ResourceLinks))
	}
	full := artifact.ResourceLinks[0]
	if full.Title == "" || full.Description == "" || full.MIMEType == "" || full.Size == nil || len(full.Annotations) == 0 || len(full.Meta) == 0 || len(full.Icons) == 0 {
		t.Fatalf("full resource link = %+v", full)
	}
	minimal := artifact.ResourceLinks[1]
	if minimal.Name == "" || minimal.URI == "" || minimal.Title != "" || minimal.Description != "" {
		t.Fatalf("minimal resource link = %+v", minimal)
	}
	request := artifact.RequestUserInput
	if request == nil || len(request.Questions) != 5 || request.AutoResolutionMS != nil {
		t.Fatalf("request = %+v, want five questions without demo expiration", request)
	}
	if !strings.HasSuffix(request.Questions[0].Options[0].Label, " (Recommended)") {
		t.Fatalf("recommended option = %+v", request.Questions[0].Options[0])
	}
	if !strings.Contains(request.Questions[1].Options[1].Label, "中文") || !request.Questions[2].IsOther {
		t.Fatalf("unicode/options/other questions = %+v", request.Questions)
	}
	if !request.Questions[3].IsOther || request.Questions[3].Options != nil {
		t.Fatalf("freeform-only question = %+v", request.Questions[3])
	}
	if !request.Questions[4].IsSecret || !strings.Contains(request.Questions[4].Question, "never a real credential") {
		t.Fatalf("secret question = %+v", request.Questions[4])
	}

	emitter, err := os.ReadFile(filepath.Join(dir, "scripts", "emit_demo.py"))
	if err != nil {
		t.Fatalf("read fixture emit_demo.py: %v", err)
	}
	skill, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read fixture SKILL.md: %v", err)
	}
	metadata, err := os.ReadFile(filepath.Join(dir, "agents", "openai.yaml"))
	if err != nil {
		t.Fatalf("read fixture openai.yaml: %v", err)
	}
	if !strings.Contains(string(skill), "execute this command exactly once") || !strings.Contains(string(skill), "do not run the emitter again") {
		t.Fatalf("SKILL.md does not guard one-shot continuation:\n%s", skill)
	}
	for _, developerReference := range []string{
		"Each supported protocol field is documented at its first use",
		"EXAMPLE_REQUEST_USER_INPUT_RESPONSE",
		"ANSWER_RESPONSE_MARKDOWN_EXAMPLE",
		"Required ResourceLink discriminator",
		"Required list containing 1 through 32 questions",
		"Optional timeout from 60000 through 240000 ms",
		`# "autoResolutionMs": 240000`,
	} {
		if !strings.Contains(string(emitter), developerReference) {
			t.Fatalf("emit_demo.py is missing developer reference %q", developerReference)
		}
	}
	markdownCommand := exec.Command(
		"python3",
		"-c",
		`import runpy, sys; values = runpy.run_path(sys.argv[1]); print(values["ANSWER_RESPONSE_MARKDOWN_EXAMPLE"])`,
		filepath.Join(dir, "scripts", "emit_demo.py"),
	)
	markdown, err := markdownCommand.Output()
	if err != nil {
		t.Fatalf("render answer Markdown example: %v", err)
	}
	if !strings.HasPrefix(string(markdown), "## Submitted `RequestUserInputResponse`\n\n```json\n") ||
		!strings.Contains(string(markdown), `"test_secret": {`) ||
		!strings.Contains(string(markdown), `"<redacted>"`) ||
		!strings.Contains(string(markdown), "## Suggested Markdown presentation") ||
		!strings.Contains(string(markdown), "- **Destination:** Documentation example / 示例") ||
		!strings.Contains(string(markdown), "- **Test secret:** Secret recorded") ||
		strings.Contains(string(markdown), "disposable-example-only") {
		t.Fatalf("answer Markdown example is malformed or leaks its secret:\n%s", markdown)
	}
	if !strings.Contains(string(skill), "Preserve every question ID and every non-secret answer exactly") ||
		!strings.Contains(string(skill), "<redacted>") ||
		!strings.Contains(string(skill), "fenced `json` block") ||
		!strings.Contains(string(skill), "Suggested Markdown presentation") ||
		!strings.Contains(string(skill), "remove one leading `user_note: ` prefix from every answer string") ||
		!strings.Contains(string(skill), "Secret recorded") {
		t.Fatalf("SKILL.md does not define the answer Markdown contract:\n%s", skill)
	}
	if !strings.Contains(string(metadata), "allow_implicit_invocation: false") {
		t.Fatalf("openai.yaml allows implicit invocation:\n%s", metadata)
	}
}

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

	dir := filepath.Join("..", "..", "template", "embed", "manager", "codex", "skills", "csgclaw-interactive-output-demo")
	emitterPath := filepath.Join(dir, "scripts", "emit_demo.py")
	command := exec.Command("python3", emitterPath, "start")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run demo emitter: %v", err)
	}
	cleaned, artifact, decodeErrors := decodeStructuredCommandOutput(string(output))
	if len(decodeErrors) != 0 {
		t.Fatalf("decode demo emitter: %v", decodeErrors)
	}
	if strings.TrimSpace(cleaned) != "## Interactive output demo - step 1 of 3\n\nChoose the workflow branch." ||
		strings.Contains(cleaned, "demo_kind") {
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
	if request == nil || len(request.Questions) != 1 || request.AutoResolutionMS != nil {
		t.Fatalf("stage 1 request = %+v, want one workflow question without demo expiration", request)
	}
	if !strings.HasSuffix(request.Questions[0].Options[0].Label, " (Recommended)") {
		t.Fatalf("recommended option = %+v", request.Questions[0].Options[0])
	}
	contextOutput, err := exec.Command("python3", emitterPath, "context", "--workflow", "bug-fix").Output()
	if err != nil {
		t.Fatalf("run demo context stage: %v", err)
	}
	contextCleaned, contextArtifact, contextErrors := decodeStructuredCommandOutput(string(contextOutput))
	if len(contextErrors) != 0 || strings.TrimSpace(contextCleaned) != "## Interactive output demo - step 2 of 3\n\nConfigure verification, destination, an optional freeform note, and presentation." ||
		strings.Contains(contextCleaned, `"verification"`) || strings.Contains(contextCleaned, `"destination"`) ||
		strings.Contains(contextCleaned, `"freeform_note"`) || strings.Contains(contextCleaned, `"presentation"`) {
		t.Fatalf("decode context stage: stdout=%q artifact=%+v errors=%v", contextCleaned, contextArtifact, contextErrors)
	}
	if len(contextArtifact.ResourceLinks) != 2 ||
		contextArtifact.ResourceLinks[0].Name != full.Name || contextArtifact.ResourceLinks[0].URI != full.URI ||
		contextArtifact.ResourceLinks[1].Name != minimal.Name || contextArtifact.ResourceLinks[1].URI != minimal.URI {
		t.Fatalf("stage 2 resource links = %+v, want the same full and minimal variants as stage 1", contextArtifact.ResourceLinks)
	}
	contextRequest := contextArtifact.RequestUserInput
	if contextRequest == nil || len(contextRequest.Questions) != 4 ||
		!strings.Contains(contextRequest.Questions[0].Options[1].Label, "中文") ||
		!contextRequest.Questions[1].IsOther || contextRequest.Questions[1].Options == nil ||
		!contextRequest.Questions[2].IsOther || contextRequest.Questions[2].Options != nil ||
		!strings.HasSuffix(contextRequest.Questions[3].Options[0].Label, " (Recommended)") ||
		!strings.Contains(contextRequest.Questions[3].Options[2].Label, "中文") {
		t.Fatalf("stage 2 request = %+v, want four mixed questions including Unicode, other, freeform-only, and Recommended", contextRequest)
	}

	confirmOutput, err := exec.Command(
		"python3", emitterPath, "confirm",
		"--workflow", "bug-fix",
		"--destination", "qa-thread",
		"--verification", "strict",
		"--presentation", "bilingual",
	).Output()
	if err != nil {
		t.Fatalf("run demo confirmation stage: %v", err)
	}
	confirmCleaned, confirmArtifact, confirmErrors := decodeStructuredCommandOutput(string(confirmOutput))
	if len(confirmErrors) != 0 || strings.TrimSpace(confirmCleaned) != "## Interactive output demo - step 3 of 3\n\nChoose the final action and optionally enter a disposable secret test value." ||
		strings.Contains(confirmCleaned, "final_action") || strings.Contains(confirmCleaned, "test_secret") {
		t.Fatalf("decode confirmation stage: stdout=%q artifact=%+v errors=%v", confirmCleaned, confirmArtifact, confirmErrors)
	}
	if len(confirmArtifact.ResourceLinks) != 0 {
		t.Fatalf("stage 3 resource links = %+v, want links only in stages 1 and 2", confirmArtifact.ResourceLinks)
	}
	confirmRequest := confirmArtifact.RequestUserInput
	if confirmRequest == nil || len(confirmRequest.Questions) != 2 ||
		!strings.HasSuffix(confirmRequest.Questions[0].Options[0].Label, " (Recommended)") ||
		!confirmRequest.Questions[1].IsSecret || confirmRequest.Questions[1].Options != nil ||
		!strings.Contains(confirmRequest.Questions[1].Question, "never a real credential") {
		t.Fatalf("stage 3 request = %+v, want final options and freeform secret", confirmRequest)
	}

	completeOutput, err := exec.Command(
		"python3", emitterPath, "complete",
		"--workflow", "bug-fix",
		"--destination", "qa-thread",
		"--verification", "strict",
		"--presentation", "bilingual",
		"--action", "execute",
	).Output()
	if err != nil {
		t.Fatalf("run demo completion stage: %v", err)
	}
	completion := string(completeOutput)
	for _, want := range []string{
		"FINAL_RECEIPT_EMITTED. STOP CURRENT TURN.",
		"## Interactive output demo complete",
		"- Workflow branch: `bug-fix`",
		"- Destination branch: `qa-thread`",
		"- Verification branch: `strict`",
		"- Presentation branch: `bilingual`",
		"- Executed action: `execute`",
		"- Secret handling: no secret value was passed to this script",
	} {
		if !strings.Contains(completion, want) {
			t.Fatalf("completion output = %q, want %q", completion, want)
		}
	}

	emitter, err := os.ReadFile(filepath.Join(dir, "scripts", "emit_demo.py"))
	if err != nil {
		t.Fatalf("read fixture emit_demo.py: %v", err)
	}
	skill, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read fixture SKILL.md: %v", err)
	}
	workflowInstructions := append([]byte(nil), skill...)
	for _, reference := range []string{"stage-2.md", "stage-3.md", "complete.md"} {
		data, readErr := os.ReadFile(filepath.Join(dir, "references", reference))
		if readErr != nil {
			t.Fatalf("read fixture reference %s: %v", reference, readErr)
		}
		workflowInstructions = append(workflowInstructions, data...)
	}
	metadata, err := os.ReadFile(filepath.Join(dir, "agents", "openai.yaml"))
	if err != nil {
		t.Fatalf("read fixture openai.yaml: %v", err)
	}
	if !strings.Contains(string(skill), "execute this command exactly once") || !strings.Contains(string(skill), "Read exactly one matching reference completely") {
		t.Fatalf("SKILL.md does not guard stage transitions:\n%s", skill)
	}
	for _, developerReference := range []string{
		"Each supported protocol field is documented at its first use",
		"The script deliberately has no response JSON input",
		"def emit_resource_links()",
		"def emit_start()",
		"def emit_context(workflow: str)",
		"def emit_confirmation(",
		"def complete(",
		"Required ResourceLink discriminator",
		"Required list containing 1 through 32 questions",
		"Optional timeout from 60000 through 240000 ms",
		`# "autoResolutionMs": 240000`,
	} {
		if !strings.Contains(string(emitter), developerReference) {
			t.Fatalf("emit_demo.py is missing developer reference %q", developerReference)
		}
	}
	for _, workflowContract := range []string{
		"It never receives or parses `RequestUserInputResponse`.",
		"The readable `## Answers` message is persisted separately by CSGClaw",
		"## Mandatory one-stage boundary",
		"Execute exactly one `emit_demo.py` command",
		"Never read or execute a later stage reference during the same turn.",
		"Tool stdout produced during this turn is never a new user response",
		"If it contains `final_action` and `test_secret`, read `references/complete.md`.",
		"context --workflow <bug-fix|new-feature|code-review|custom>",
		"confirm --workflow <bug-fix|new-feature|code-review|custom> --destination",
		"--verification <standard|strict|fast|unspecified> --presentation <concise|detailed|bilingual|unspecified>",
		"complete --workflow <bug-fix|new-feature|code-review|custom> --destination",
		"Never include received response JSON in a user-visible response.",
		"Never repeat a secret answer value",
	} {
		if !strings.Contains(string(workflowInstructions), workflowContract) {
			t.Fatalf("skill instructions are missing multi-stage contract %q:\n%s", workflowContract, workflowInstructions)
		}
	}
	if !strings.Contains(string(metadata), "allow_implicit_invocation: false") {
		t.Fatalf("openai.yaml allows implicit invocation:\n%s", metadata)
	}
}

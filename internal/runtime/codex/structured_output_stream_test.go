package codex

import "testing"

func TestAssistantStructuredOutputStreamPassesOrdinaryTextIncrementally(t *testing.T) {
	t.Parallel()

	stream := newAssistantStructuredOutputStream()
	if got := stream.append("ordinary text"); got != "ordinary text" {
		t.Fatalf("append() = %q, want ordinary text", got)
	}
	if got := stream.append("\nnext"); got != "\nnext" {
		t.Fatalf("append() = %q, want newline and next line", got)
	}
	if got := stream.finish("ordinary text\nnext"); got != "" {
		t.Fatalf("finish() = %q, want no duplicate suffix", got)
	}
}

func TestAssistantStructuredOutputStreamPreservesUTF8Text(t *testing.T) {
	t.Parallel()

	stream := newAssistantStructuredOutputStream()
	var got string
	for _, delta := range []string{"你好", "，世界", "\n继续处理"} {
		got += stream.append(delta)
	}
	if got != "你好，世界\n继续处理" {
		t.Fatalf("streamed text = %q, want valid UTF-8 text", got)
	}
	if remainder := stream.finish("你好，世界\n继续处理"); remainder != "" {
		t.Fatalf("finish() = %q, want no duplicate suffix", remainder)
	}
}

func TestAssistantStructuredOutputStreamPreservesUTF8BeforeControlSuffix(t *testing.T) {
	t.Parallel()

	stream := newAssistantStructuredOutputStream()
	var got string
	for _, delta := range []string{
		"请选择一个选项",
		"\n:::csgclaw-",
		`output::request_user_input {"questions":[]}`,
	} {
		got += stream.append(delta)
	}
	if got != "请选择一个选项" {
		t.Fatalf("streamed text = %q, want UTF-8 text without control suffix", got)
	}
}

func TestAssistantStructuredOutputStreamSuppressesChunkedControlSuffix(t *testing.T) {
	t.Parallel()

	stream := newAssistantStructuredOutputStream()
	var got string
	for _, delta := range []string{
		"Need your choice",
		"\n:",
		"::csgclaw-",
		`output::request_user_input {"questions":[]}`,
	} {
		got += stream.append(delta)
	}
	if got != "Need your choice" {
		t.Fatalf("streamed text = %q, want sanitized assistant text", got)
	}
	if remainder := stream.finish("Need your choice"); remainder != "" {
		t.Fatalf("finish() = %q, want no control-record remainder", remainder)
	}
}

func TestAssistantStructuredOutputStreamReleasesFalsePrefix(t *testing.T) {
	t.Parallel()

	stream := newAssistantStructuredOutputStream()
	var got string
	for _, delta := range []string{"first", "\n::csg", "claw-not-control"} {
		got += stream.append(delta)
	}
	if got != "first\n::csgclaw-not-control" {
		t.Fatalf("streamed text = %q, want false prefix preserved", got)
	}
}

package codex

import (
	"testing"

	"csgclaw/internal/activity"
)

func TestLegacyBackgroundShellOutput(t *testing.T) {
	const link = `::csgclaw-output::resource_link {"type":"resource_link","name":"docs","uri":"https://example.com/docs"}`
	const question = `::csgclaw-output::request_user_input {"questions":[{"id":"next","header":"Next","question":"Continue?","options":[{"label":"Yes","description":"Proceed."}]}]}`
	for _, tc := range []struct {
		name, partial string
		steps         []string
		wantLinks     int
		wantQuestions int
	}{
		{"background exit", link, []string{"exit"}, 1, 0},
		{"transcript only", "waiting", []string{"exit"}, 1, 0},
		{"unsuccessful write leaves process running", link, []string{"write error", "empty poll"}, 1, 0},
		{"exit before last poll", link, []string{"exit", "poll"}, 1, 0},
		{"last poll before full transcript", link, []string{"poll", "exit with question", "exit with question"}, 1, 1},
		{"unsuccessful process", link, []string{"failed exit", "failed poll"}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, live, sink := testAppServerEventAdapter(t)
			send := func(method string, params map[string]any) {
				manager.handleAppServerNotification("runtime-1", live, appServerNotification{Method: method, Params: mustJSONRaw(t, params)})
			}
			call := func(id, name, args, output string) {
				send("codex/response_item", map[string]any{"type": "function_call", "call_id": id, "name": name, "arguments": args})
				send("codex/response_item", map[string]any{"type": "function_call_output", "call_id": id, "output": output})
			}
			exit := func(status, output string) {
				send("codex/event", map[string]any{
					"type": "exec_command_end", "call_id": "run", "process_id": "55", "source": "unified_exec_startup",
					"status": status, "aggregated_output": output,
				})
			}
			call("run", "exec_command", `{"cmd":"./script.sh","tty":false}`, "Wall time: 1 second\nProcess running with session ID 55\nOutput:\n"+tc.partial)
			for _, step := range tc.steps {
				switch step {
				case "exit":
					exit("completed", link)
				case "exit with question":
					exit("completed", link+"\n"+question)
				case "failed exit":
					exit("failed", link)
				case "write error":
					call("write", "write_stdin", `{"session_id":55,"chars":"hello"}`, "write_stdin failed: stdin is closed for this session; rerun exec_command with tty=true to keep stdin open")
				case "empty poll":
					call("poll", "write_stdin", `{"session_id":55,"chars":""}`, "Process exited with code 0\nOutput:\n")
				case "poll":
					call("poll", "write_stdin", `{"session_id":55,"chars":""}`, "Process exited with code 0\nOutput:\n"+link)
				case "failed poll":
					call("poll", "write_stdin", `{"session_id":55,"chars":""}`, "Process exited with code 1\nOutput:\n"+link)
				}
			}
			var links, questions, runResults int
			for _, event := range sink.snapshot() {
				switch event.Kind {
				case SessionEventStructuredOutput:
					artifact := event.Payload.(activity.StructuredOutputArtifact)
					links += len(artifact.ResourceLinks)
					if artifact.RequestUserInput != nil {
						questions++
					}
				case SessionEventToolCallUpdate:
					if event.ToolCallID == "run" {
						runResults++
					}
					if event.ToolCallID == "write" && event.ToolStatus != "failed" {
						t.Errorf("write status = %q, want failed", event.ToolStatus)
					}
				}
			}
			if runResults != 1 || links != tc.wantLinks || questions != tc.wantQuestions {
				t.Fatalf("got %d initial tool results, %d links and %d questions; want 1, %d, %d", runResults, links, questions, tc.wantLinks, tc.wantQuestions)
			}
		})
	}
}

func TestLegacyBackgroundShellReusesPTYNumber(t *testing.T) {
	const link = `::csgclaw-output::resource_link {"type":"resource_link","name":"docs","uri":"https://example.com/docs"}`
	manager, live, sink := testAppServerEventAdapter(t)
	run := func(id string) {
		manager.handleLegacyResponseItemEvent("runtime-1", live, map[string]any{
			"type": "function_call", "name": "exec_command", "call_id": id, "arguments": `{"cmd":"./script.sh"}`,
		})
		manager.handleLegacyResponseItemEvent("runtime-1", live, map[string]any{
			"type": "function_call_output", "call_id": id, "output": "Process running with session ID 55\nOutput:\n" + link,
		})
	}
	exit := func(id, status string) {
		manager.handleLegacyAppServerEvent("runtime-1", live, map[string]any{
			"type": "exec_command_end", "call_id": id, "process_id": "55", "status": status, "aggregated_output": link,
		})
	}
	run("first")
	exit("first", "failed")
	run("second")
	exit("first", "failed")
	exit("second", "completed")
	count := 0
	for _, event := range sink.snapshot() {
		if event.Kind == SessionEventStructuredOutput {
			count++
			if event.ToolCallID != "second" {
				t.Fatalf("unexpected structured output from %q", event.ToolCallID)
			}
		}
	}
	if count != 1 {
		t.Fatalf("got %d structured outputs, want one from the second command", count)
	}
}

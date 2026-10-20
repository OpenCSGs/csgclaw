package dsh

import (
	"bufio"
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/modelusage"
	agentruntime "csgclaw/internal/runtime"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestContextBridgeIncludesCompactionUsage(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node required for the managed bridge test")
	}
	script := contextBridgeModule + `
const handlers = new Map();
apply({on: (name, handler) => handlers.set(name, handler)});
const session = {header: {id: 's'}};
const emit = (type, seq, usage) => handlers.get('session/event')(session, {type, seq, data: {usage}});
emit('assistant/message', 1, {inputTokens:100, outputTokens:20, totalTokens:120});
emit('compaction/summary', 2, {inputTokens:200, outputTokens:30, totalTokens:230});
emit('compaction/summary', 2, {inputTokens:200, outputTokens:30, totalTokens:230});
emit('compaction/summary', 3, undefined);
`
	output, err := exec.Command(node, "--input-type=module", "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("bridge: %v %s", err, output)
	}
	var last modelusage.Usage
	turn := &activeTurn{request: contract.TurnRequest{ID: "t"}, sink: contract.EventSinkFunc(func(_ context.Context, e contract.TurnEvent) error {
		last, _ = e.Activity.Payload.(modelusage.Usage)
		return nil
	})}
	proc := &process{profile: agentruntime.Profile{ModelID: "auto"}, active: map[string]*activeTurn{"s": turn}}
	r := &Runtime{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		var note struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &note); err != nil {
			t.Fatal(err)
		}
		r.handleNotification(proc, notification{Method: note.Method, Params: note.Params})
	}
	if !last.Known || !last.Partial || last.Total != 350 {
		t.Fatalf("bridge consumption=%+v", last)
	}
}

func TestTokenUsageNotificationsAggregateAndDeduplicate(t *testing.T) {
	var events []contract.TurnEvent
	turn := &activeTurn{request: contract.TurnRequest{ID: "t"}, sink: contract.EventSinkFunc(func(_ context.Context, e contract.TurnEvent) error { events = append(events, e); return nil })}
	proc := &process{profile: agentruntime.Profile{ModelID: "auto"}, active: map[string]*activeTurn{"s": turn}}
	r := &Runtime{}
	send := func(session, id, usage string) {
		raw := json.RawMessage(`{"sessionId":"` + session + `","update":{"sessionUpdate":"token_usage","requestId":"` + id + `","usage":` + usage + `}}`)
		r.handleNotification(proc, notification{Method: "csgclaw/context", Params: raw})
	}
	send("s", "1", `{"inputTokens":100,"outputTokens":20,"totalTokens":120}`)
	send("s", "1", `{"inputTokens":100,"outputTokens":20,"totalTokens":120}`)
	send("other", "1", `{"inputTokens":900,"outputTokens":20}`)
	send("s", "2", `{"inputTokens":200,"outputTokens":30,"totalTokens":230}`)
	send("s", "3", `null`)
	got := events[len(events)-1].Activity.Payload.(modelusage.Usage)
	if len(events) != 3 || got.Total != 350 || got.Input != 300 || got.Output != 50 || !got.Partial || got.ModelID != "auto" {
		t.Fatalf("events=%d usage=%+v", len(events), got)
	}
}

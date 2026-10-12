package dsh

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/agentengine/contract"
	agentruntime "csgclaw/internal/runtime"
)

func TestSelectReasoningEffort(t *testing.T) {
	for _, test := range []struct {
		name      string
		desired   string
		available []string
		want      string
	}{
		{name: "minimal lifts to low", desired: "minimal", available: []string{"off", "low", "high", "max"}, want: "low"},
		{name: "medium lifts to high", desired: "medium", available: []string{"off", "low", "high", "max"}, want: "high"},
		{name: "xhigh lifts to max", desired: "xhigh", available: []string{"off", "low", "high", "max"}, want: "max"},
		{name: "supported low", desired: "low", available: []string{"off", "low", "high", "max"}, want: "low"},
		{name: "supported high", desired: "high", available: []string{"off", "low", "high", "max"}, want: "high"},
		{name: "supported xhigh", desired: "xhigh", available: []string{"high", "xhigh", "max"}, want: "xhigh"},
		{name: "supported medium", desired: "medium", available: []string{"low", "medium", "high"}, want: "medium"},
		{name: "supported minimal", desired: "minimal", available: []string{"minimal", "low"}, want: "minimal"},
		{name: "lower when closer", desired: "medium", available: []string{"low", "max"}, want: "low"},
		{name: "highest when no higher", desired: "xhigh", available: []string{"off", "low", "high"}, want: "high"},
		{name: "single positive level", desired: "minimal", available: []string{"off", "high"}, want: "high"},
		{name: "normalized profile", desired: " XHIGH ", available: []string{"off", "low", "high", "max"}, want: "max"},
		{name: "disable alias", desired: "none", available: []string{"off", "low"}, want: "off"},
		{name: "native none", desired: "off", available: []string{"none", "low"}, want: "none"},
		{name: "never enable disabled reasoning", desired: "none", available: []string{"low", "high"}},
		{name: "never disable explicit reasoning", desired: "minimal", available: []string{"off"}},
		{name: "no advertised levels", desired: "xhigh"},
		{name: "unknown levels cannot be ranked", desired: "medium", available: []string{"standard", "ultra"}},
		{name: "unknown request cannot be ranked", desired: "ultra", available: []string{"low", "high", "max"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := selectReasoningEffort(reasoningOptions(test.available...), test.desired)
			if got != test.want || ok != (test.want != "") {
				t.Fatalf("selectReasoningEffort() = %q, %v; want %q, %v", got, ok, test.want, test.want != "")
			}
		})
	}

	t.Run("grouped opaque values", func(t *testing.T) {
		options := []json.RawMessage{json.RawMessage(`{"id":"reasoning_effort","options":[{"group":"thinking","options":[{"value":"budget-low","name":"Low"},{"value":"budget-max","name":"Max"}]}]}`)}
		if got, ok := selectReasoningEffort(options, "xhigh"); !ok || got != "budget-max" {
			t.Fatalf("selectReasoningEffort() = %q, %v; want budget-max", got, ok)
		}
	})
}

func TestSessionMapsReasoningEffortOnNewAndResume(t *testing.T) {
	for _, resume := range []bool{false, true} {
		method := "session/new"
		if resume {
			method = "session/resume"
		}
		for _, test := range []struct{ effort, want string }{
			{effort: "minimal", want: "low"},
			{effort: "medium", want: "high"},
			{effort: "xhigh", want: "max"},
			{effort: "high", want: "high"},
			{effort: "none", want: "off"},
			{effort: "auto"},
			{effort: ""},
		} {
			t.Run(method+"/"+test.effort, func(t *testing.T) {
				// The initial model supports xhigh; selecting the configured model
				// returns a different set of supported efforts.
				initial := append(modelOptions(), reasoningOptions("off", "medium", "xhigh")...)
				updated := append(modelOptions(), reasoningOptions("off", "low", "high", "max")...)
				client, frames := reasoningACPClient(t, initial, updated)
				proc := &process{
					client: client, root: t.TempDir(), workspace: t.TempDir(),
					profile: agentruntime.Profile{ModelID: "test-model", ReasoningEffort: test.effort},
					meta:    runtimeMetadata{Sessions: map[string]string{}},
					ready:   map[string]bool{},
				}
				if resume {
					proc.meta.Sessions["room-1"] = "session-1"
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				conversation := &conversation{}
				request := contract.TurnRequest{ConversationKey: "room-1"}
				sessionID, err := conversation.ensureSession(ctx, proc, request)
				if err != nil || sessionID != "session-1" || !proc.ready[sessionID] {
					t.Fatalf("ensureSession() = %q, %v; ready = %v", sessionID, err, proc.ready)
				}
				if got := <-frames; got.Method != method {
					t.Fatalf("first request = %+v, want %s", got, method)
				}
				if got := <-frames; got.Method != "session/set_config_option" || got.Params.ConfigID != "model" || got.Params.Value != `["deepseek-official","test-model"]` {
					t.Fatalf("model request = %+v", got)
				}
				if test.want != "" {
					if got := <-frames; got.Method != "session/set_config_option" || got.Params.ConfigID != "reasoning_effort" || got.Params.Value != test.want || got.Params.SessionID != sessionID {
						t.Fatalf("reasoning request = %+v, want %q", got, test.want)
					}
				}
				if proc.profile.ReasoningEffort != test.effort {
					t.Fatalf("profile effort changed to %q", proc.profile.ReasoningEffort)
				}
				if _, err := conversation.ensureSession(ctx, proc, request); err != nil {
					t.Fatalf("ensureSession() on ready session: %v", err)
				}
				if len(frames) != 0 {
					t.Fatalf("unexpected ACP requests: %d", len(frames))
				}
			})
		}
	}
}

func TestConfigureSessionReasoningOptions(t *testing.T) {
	for _, test := range []struct {
		name    string
		updated []json.RawMessage
		want    string
		wantErr bool
	}{
		{name: "legacy empty response uses initial options", want: "max"},
		{name: "selected model has no reasoning selector", updated: modelOptions(), wantErr: true},
		{name: "selected model has no positive effort", updated: append(modelOptions(), reasoningOptions("off")...), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			initial := append(modelOptions(), reasoningOptions("off", "low", "high", "max")...)
			client, frames := reasoningACPClient(t, initial, test.updated)
			proc := &process{client: client, profile: agentruntime.Profile{ModelID: "test-model", ReasoningEffort: "xhigh"}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := configureSession(ctx, proc, "session-1", initial)
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "does not advertise reasoning effort") {
					t.Fatalf("configureSession() = %v, want unsupported effort error", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			<-frames // model selection
			if test.want != "" {
				if got := <-frames; got.Params.Value != test.want {
					t.Fatalf("reasoning request = %+v, want %q", got, test.want)
				}
			}
			if len(frames) != 0 {
				t.Fatal("unexpected reasoning request")
			}
		})
	}
}

func reasoningOptions(values ...string) []json.RawMessage {
	items := make([]map[string]string, 0, len(values))
	for _, value := range values {
		items = append(items, map[string]string{"value": value, "name": value})
	}
	data, _ := json.Marshal(map[string]any{"id": "reasoning_effort", "type": "select", "options": items})
	return []json.RawMessage{data}
}

func modelOptions() []json.RawMessage {
	return []json.RawMessage{json.RawMessage(`{"id":"model","type":"select","options":[{"value":"[\"deepseek-official\",\"test-model\"]","name":"Test Model"}]}`)}
}

type reasoningACPFrame struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
	Params struct {
		SessionID string `json:"sessionId"`
		ConfigID  string `json:"configId"`
		Value     string `json:"value"`
	} `json:"params"`
}

func reasoningACPClient(t *testing.T, initial, updated []json.RawMessage) (*acpClient, <-chan reasoningACPFrame) {
	t.Helper()
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()
	t.Cleanup(func() {
		_ = clientWriter.Close()
		_ = serverReader.Close()
		_ = serverWriter.Close()
		_ = clientReader.Close()
	})
	frames := make(chan reasoningACPFrame, 8)
	go func() {
		scanner := bufio.NewScanner(serverReader)
		for scanner.Scan() {
			var frame reasoningACPFrame
			if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
				return
			}
			frames <- frame
			options := updated
			if frame.Method == "session/new" || frame.Method == "session/resume" {
				options = initial
			}
			if err := json.NewEncoder(serverWriter).Encode(map[string]any{
				"jsonrpc": "2.0", "id": frame.ID,
				"result": sessionResult{SessionID: "session-1", ConfigOptions: options},
			}); err != nil {
				return
			}
		}
	}()
	return newACPClient(clientReader, clientWriter), frames
}

package llm

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"
)

func TestUsageObserverPreservesBytesAndChecksFinalUsage(t *testing.T) {
	for _, tc := range []struct {
		name, body   string
		stream, want bool
	}{
		{"responses", "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":10,\"total_tokens\":110}}}\n\n", true, true},
		{"responses-missing", "data: {\"type\":\"response.completed\",\"response\":{}}\n\n", true, false},
		{"responses-incomplete", "data: {\"type\":\"response.incomplete\",\"response\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":10}}}\n\n", true, true},
		{"chat-late-usage", "data: {\"choices\":[{\"finish_reason\":\"stop\"}],\"usage\":null}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":14,\"completion_tokens\":144,\"total_tokens\":158}}\n\ndata: [DONE]\n\n", true, true},
		{"chat-missing", "data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", true, false},
		{"messages-late-input", "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":14,\"output_tokens\":164}}\n\ndata: {\"type\":\"message_stop\"}\n\n", true, true},
		{"messages-placeholder-only", "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\ndata: {\"type\":\"message_stop\"}\n\n", true, false},
		{"json-responses", `{"usage":{"input_tokens":100,"output_tokens":10}}`, false, true},
		{"json-chat-zero", `{"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`, false, true},
		{"json-missing", `{"usage":null}`, false, false},
		{"json-malformed-usage", `{"usage":{"input_tokens":-1,"output_tokens":10}}`, false, false},
		{"multiline-crlf", "data: {\"type\":\"response.completed\",\r\ndata: \"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\r\n\r\n", true, true},
		{"eof-without-blank-line", "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reported []bool
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(iotest.OneByteReader(strings.NewReader(tc.body)))}
			if tc.stream {
				resp.Header.Set("Content-Type", "text/event-stream")
			}
			observeResponseUsage(resp, func(known bool) { reported = append(reported, known) })
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if err = resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if string(body) != tc.body {
				t.Fatal("response bytes changed")
			}
			if len(reported) != 1 || reported[0] != tc.want {
				t.Fatalf("reports=%v want=%v", reported, tc.want)
			}
		})
	}
}

func TestUsageObserverReportsBeforeDeliveringTerminalBytes(t *testing.T) {
	for _, body := range []string{
		"data: {\"type\":\"response.completed\",\"response\":{}}\n\n",
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n",
	} {
		called := false
		resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
		observeResponseUsage(resp, func(bool) { called = true })
		buf := make([]byte, len(body))
		n, err := resp.Body.Read(buf)
		if err != nil || n != len(body) || !called {
			t.Fatalf("terminal delivered before usage observation: n=%d err=%v called=%v", n, err, called)
		}
		_ = resp.Body.Close()
	}
}

func TestUsageObserverIgnoresRejectedAttemptsAndMarksInterruptedBody(t *testing.T) {
	called := false
	resp := &http.Response{StatusCode: 401, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`))}
	observeResponseUsage(resp, func(bool) { called = true })
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if called {
		t.Fatal("auth retry counted as missing usage")
	}
	var reports []bool
	resp = &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))}
	observeResponseUsage(resp, func(known bool) { reports = append(reports, known) })
	_, _ = resp.Body.Read(make([]byte, 8))
	_ = resp.Body.Close()
	if len(reports) != 1 || reports[0] {
		t.Fatalf("interrupted=%v", reports)
	}
}

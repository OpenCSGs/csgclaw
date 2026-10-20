package llm

import (
	"csgclaw/internal/modelusage"
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
			observeResponseUsage(resp, false, func(usage *modelusage.Usage) { reported = append(reported, usage != nil) })
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
		observeResponseUsage(resp, false, func(*modelusage.Usage) { called = true })
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
	observeResponseUsage(resp, false, func(*modelusage.Usage) { called = true })
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if called {
		t.Fatal("auth retry counted as missing usage")
	}
	var reports []bool
	resp = &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"))}
	observeResponseUsage(resp, false, func(usage *modelusage.Usage) { reports = append(reports, usage != nil) })
	_, _ = resp.Body.Read(make([]byte, 8))
	_ = resp.Body.Close()
	if len(reports) != 1 || reports[0] {
		t.Fatalf("interrupted=%v", reports)
	}
}

func TestUsageObserverReturnsFinalCountersExactlyOnce(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		stream     bool
	}{
		{"responses", `{"usage":{"input_tokens":100,"output_tokens":10,"input_tokens_details":{"cached_tokens":40},"output_tokens_details":{"reasoning_tokens":5}}}`, false},
		{"responses-stream", "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":10,\"input_tokens_details\":{\"cached_tokens\":40},\"output_tokens_details\":{\"reasoning_tokens\":5}}}}\n\ndata: [DONE]\n\n", true},
		{"chat-stream", "data: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\ndata: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":10,\"prompt_tokens_details\":{\"cached_tokens\":40},\"completion_tokens_details\":{\"reasoning_tokens\":5}}}\n\ndata: [DONE]\n\n", true},
		{"messages-stream", "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":60,\"output_tokens\":0,\"cache_read_input_tokens\":40}}}\n\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":10,\"reasoning_tokens\":5}}\n\ndata: {\"type\":\"message_stop\"}\n\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			var got *modelusage.Usage
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(iotest.OneByteReader(strings.NewReader(tc.body)))}
			if tc.stream {
				resp.Header.Set("Content-Type", "text/event-stream")
			}
			observeResponseUsage(resp, false, func(u *modelusage.Usage) { count++; got = u })
			_, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if count != 1 || got == nil || got.Input != 100 || got.Output != 10 || got.Total != 110 || got.Cached != 40 || got.Reasoning != 5 {
				t.Fatalf("count=%d usage=%+v", count, got)
			}
		})
	}
}

func TestUsageObserverStreamHintOnlyAppliesWithoutContentType(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		requestedStream         bool
	}{
		{"missing-stream-header", "", "data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":10}}\n\ndata: [DONE]\n\n", true},
		{"missing-json-header", "", `{"usage":{"input_tokens":100,"output_tokens":10}}`, false},
		{"explicit-json-overrides-stream-request", "application/json", `{"usage":{"input_tokens":100,"output_tokens":10}}`, true},
		{"explicit-stream-overrides-json-request", "text/event-stream", "data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":10}}\n\ndata: [DONE]\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			var got *modelusage.Usage
			resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}
			resp.Header.Set("Content-Type", tc.contentType)
			observeResponseUsage(resp, tc.requestedStream, func(u *modelusage.Usage) { count++; got = u })
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if count != 1 || got == nil || got.Total != 110 || string(body) != tc.body {
				t.Fatalf("reports=%d usage=%+v bytes preserved=%v", count, got, string(body) == tc.body)
			}
		})
	}
}

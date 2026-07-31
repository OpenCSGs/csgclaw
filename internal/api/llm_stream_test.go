package api

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/llm"
)

func TestHandleBotLLMChatCompletionsFlushesSSEBeforeCompletion(t *testing.T) {
	firstChunkSent := make(chan struct{})
	releaseUpstream := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseUpstream) }) }

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Upstream-Test", "chat-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		close(firstChunkSent)
		<-releaseUpstream
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(func() {
		release()
		upstream.Close()
	})

	agentSvc := mustNewSeededService(t, []agent.Agent{
		{
			ID:   agent.ManagerUserID,
			Name: agent.ManagerName,
			Role: agent.RoleManager,
			AgentProfile: agent.AgentProfile{
				Provider:        agent.ProviderAPI,
				BaseURL:         upstream.URL + "/v1",
				APIKey:          "sk-test",
				ModelID:         "stream-model",
				ProfileComplete: true,
			},
			ProfileComplete: true,
		},
	})
	handler := &Handler{llm: llm.NewService(config.ModelConfig{}, agentSvc)}
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.handleBotLLMChatCompletions(w, r, agent.ManagerUserID)
	}))
	t.Cleanup(func() {
		release()
		bridge.Close()
	})

	request, err := http.NewRequest(
		http.MethodPost,
		bridge.URL+"/v1/chat/completions",
		bytes.NewBufferString(`{"model":"client-model","messages":[],"stream":true}`),
	)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	type responseResult struct {
		resp *http.Response
		err  error
	}
	responseCh := make(chan responseResult, 1)
	go func() {
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
		responseCh <- responseResult{resp: resp, err: err}
	}()

	select {
	case <-firstChunkSent:
	case <-time.After(time.Second):
		t.Fatal("upstream did not send the first SSE chunk")
	}

	var response *http.Response
	select {
	case result := <-responseCh:
		if result.err != nil {
			t.Fatalf("bridge request error = %v", result.err)
		}
		response = result.resp
	case <-time.After(time.Second):
		release()
		t.Fatal("bridge buffered the upstream SSE response until completion")
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := response.Header.Get("X-Upstream-Test"); got != "chat-stream" {
		t.Fatalf("X-Upstream-Test = %q, want chat-stream", got)
	}

	reader := bufio.NewReader(response.Body)
	type lineResult struct {
		line string
		err  error
	}
	firstLineCh := make(chan lineResult, 1)
	go func() {
		line, err := reader.ReadString('\n')
		firstLineCh <- lineResult{line: line, err: err}
	}()
	select {
	case result := <-firstLineCh:
		if result.err != nil {
			t.Fatalf("read first SSE chunk error = %v", result.err)
		}
		if result.line != "data: first\n" {
			t.Fatalf("first SSE line = %q, want %q", result.line, "data: first\n")
		}
	case <-time.After(time.Second):
		release()
		t.Fatal("bridge wrote headers but did not flush the first SSE chunk")
	}

	release()
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !strings.Contains(string(rest), "data: [DONE]") {
		t.Fatalf("remaining SSE body = %q, want DONE event", rest)
	}
}

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/activity"
	"csgclaw/internal/agentengine"
	agent "csgclaw/internal/agentengine/agents"
)

const sessionQuestionPath = "/api/v1/agents/agent-alpha/sessions/planning"
const sessionQuestionID = "question-1"

func sessionQuestionEvent() agentengine.TurnEvent {
	return agentengine.TurnEvent{
		Kind: agentengine.TurnEventInteractionRequest,
		Interaction: &agentengine.InteractionRequest{
			ID: sessionQuestionID, Kind: agentengine.InteractionUserInput,
			Payload: activity.UserInputSnapshot{
				ID: sessionQuestionID, Status: activity.UserInputStatusPending,
				Questions: []activity.UserInputQuestionSnapshot{{
					ID: "source", Header: "Source", Question: "Which documents?",
					Options: []activity.UserInputOptionSnapshot{{Label: "PDF", Description: "Local files"}},
				}},
			},
		},
	}
}

func TestAgentSessionUserInput(t *testing.T) {
	for _, action := range []string{"stream", "sync", "cancel"} {
		t.Run(action, func(t *testing.T) {
			resume := make(chan struct{})
			pending := make(chan struct{})
			engine := &fakeSessionEngine{}
			engine.run = func(ctx context.Context, _ string, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
				if err := sink.Emit(ctx, sessionQuestionEvent()); err != nil {
					return agentengine.TurnResult{Status: agentengine.TurnFailed, Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeFailed, Message: err.Error()}}
				}
				close(pending)
				select {
				case <-ctx.Done():
					return agentengine.TurnResult{Status: agentengine.TurnCanceled, Error: &agentengine.TurnError{Code: agentengine.ErrorCanceled, Message: ctx.Err().Error()}}
				case <-resume:
				}
				resolutions := engine.client.Resolutions()
				if len(resolutions) != 1 || resolutions[0].Value.ConversationKey != request.ConversationKey || resolutions[0].Value.InteractionID != sessionQuestionID {
					t.Errorf("runtime received unexpected answers: %+v", resolutions)
					return agentengine.TurnResult{Status: agentengine.TurnFailed}
				}
				answer := strings.Join(resolutions[0].Value.Answers["source"].Values, ",")
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Using " + answer}
			}
			handler, _, bindings, _ := newAgentSessionTestHandler(t, []agent.Agent{
				sessionCodexAgent("agent-alpha", "Alpha"), sessionCodexAgent("agent-beta", "Beta"),
			}, engine, "")
			for _, other := range []struct{ agent, session string }{{"agent-alpha", "other"}, {"agent-beta", "planning"}} {
				if _, err := bindings.GetOrCreate(other.agent, other.session); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(handler.Routes())
			t.Cleanup(server.Close)
			var stream *bufio.Reader
			var syncResponse <-chan *httptest.ResponseRecorder
			if action == "sync" {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				syncResponse = startSessionRequest(ctx, server.Config.Handler, sessionQuestionPath+"/responses", `{"input":"Plan","user_input":true}`)
				select {
				case <-pending:
				case <-ctx.Done():
					t.Fatal("synchronous turn did not request an answer")
				}
			} else {
				stream = openSessionQuestionStream(t, server)
			}
			const answers = `{"answers":{"source":{"answers":["PDF"]}}}`
			const interactionPath = "/interactions/" + sessionQuestionID + "/respond"
			body := sessionInteractionHTTP(t, server, http.MethodGet, sessionQuestionPath+"/interactions", "", http.StatusOK)
			if !strings.Contains(body, sessionQuestionID) || !strings.Contains(body, "Which documents?") {
				t.Fatalf("pending interactions: %s", body)
			}
			for _, other := range []string{"/api/v1/agents/agent-alpha/sessions/other", "/api/v1/agents/agent-beta/sessions/planning"} {
				body := sessionInteractionHTTP(t, server, http.MethodGet, other+"/interactions", "", http.StatusOK)
				if strings.Contains(body, sessionQuestionID) {
					t.Fatalf("session %s can read another session's question", other)
				}
				sessionInteractionHTTP(t, server, http.MethodPost, other+interactionPath, answers, http.StatusNotFound)
			}
			if action == "cancel" {
				sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/responses/cancel", "", http.StatusNoContent)
				sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+interactionPath, answers, http.StatusGone)
				if len(engine.client.Resolutions()) != 0 {
					t.Fatal("runtime received an answer after cancellation")
				}
				return
			}
			sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+interactionPath, `{"answers":{"other":{"answers":["PDF"]}}}`, http.StatusBadRequest)
			sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+interactionPath, answers, http.StatusNoContent)
			sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+interactionPath, answers, http.StatusConflict)
			close(resume)
			if action == "sync" {
				response := <-syncResponse
				if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Using PDF") {
					t.Fatalf("continued JSON: HTTP %d %s", response.Code, response.Body.String())
				}
			} else {
				rest, err := io.ReadAll(stream)
				if err != nil || !strings.Contains(string(rest), "Using PDF") || !strings.Contains(string(rest), "event: message_stop") || strings.Contains(string(rest), "event: error") {
					t.Fatalf("continued response: %s, error=%v", rest, err)
				}
			}
			if calls := engine.client.Calls(); len(calls) != 1 {
				t.Fatalf("runtime received %d turns, want one", len(calls))
			}
		})
	}
}

func sessionInteractionHTTP(t *testing.T, server *httptest.Server, method, path, body string, wantStatus int) string {
	t.Helper()
	status, payload := sessionFileHTTP(t, server, method, path, "application/json", strings.NewReader(body), "Bearer test-server-token")
	if status != wantStatus {
		t.Fatalf("%s %s returned HTTP %d, want %d: %s", method, path, status, wantStatus, payload)
	}
	return string(payload)
}

func openSessionQuestionStream(t *testing.T, server *httptest.Server) *bufio.Reader {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+sessionQuestionPath+"/responses", strings.NewReader(`{"input":"Build an assistant","stream":true,"user_input":true}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-server-token")
	req.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("open response: HTTP %d %s", response.StatusCode, body)
	}
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read request_user_input: %v", err)
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var event struct {
			Type          string                               `json:"type"`
			InteractionID string                               `json:"interaction_id"`
			Questions     []activity.UserInputQuestionSnapshot `json:"questions"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("decode SSE event %q: %v", data, err)
		}
		if event.Type == "error" || event.Type == "message_stop" {
			t.Fatalf("response finished before the question: %s", data)
		}
		if event.Type != "request_user_input" {
			continue
		}
		if event.InteractionID != sessionQuestionID || len(event.Questions) != 1 || event.Questions[0].ID != "source" ||
			event.Questions[0].Question != "Which documents?" || len(event.Questions[0].Options) != 1 || event.Questions[0].Options[0].Label != "PDF" {
			t.Fatalf("unexpected question event: %s", data)
		}
		return reader
	}
}

func sessionDetachedQuestionEvent() agentengine.TurnEvent {
	return agentengine.TurnEvent{
		Kind: agentengine.TurnEventOutputItem,
		Output: &agentengine.OutputItem{
			Kind: agentengine.OutputItemRequestUserInput,
			Payload: activity.RequestUserInputArgs{Questions: []activity.RequestUserInputQuestion{
				{ID: "source", Header: "Source", Question: "Which documents?", Options: []activity.RequestUserInputOption{{Label: "PDF"}}},
				{ID: "password", Header: "Password", Question: "Password?", IsSecret: true, IsOther: true},
			}},
		},
	}
}

func TestAgentSessionDetachedUserInput(t *testing.T) {
	for _, action := range []string{"stream", "json", "skip"} {
		t.Run(action, func(t *testing.T) {
			engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
				if sessionTurnText(request) == "Plan" {
					err := sink.Emit(ctx, sessionDetachedQuestionEvent())
					if err != nil {
						t.Errorf("emit detached question: %v", err)
					}
				}
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Ready"}
			}}
			server, _ := sessionOutputTestServer(t, engine, "")
			original := `{"input":"Plan","stream":true,"user_input":true}`
			if action == "json" {
				original = `{"input":"Plan","user_input":true}`
			}
			first := sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/responses", original, http.StatusOK)
			if action != "json" && (!strings.Contains(first, "event: request_user_input") || !strings.Contains(first, "event: message_stop")) {
				t.Fatalf("initial response: %s", first)
			}
			body := sessionInteractionHTTP(t, server, http.MethodGet, sessionQuestionPath+"/interactions", "", http.StatusOK)
			var pending struct {
				Interactions []struct {
					ID       string `json:"interaction_id"`
					Detached bool   `json:"detached"`
				} `json:"interactions"`
			}
			if err := json.Unmarshal([]byte(body), &pending); err != nil || len(pending.Interactions) != 1 || !pending.Interactions[0].Detached || pending.Interactions[0].ID == "" {
				t.Fatalf("detached question after initial response: %s, %v", body, err)
			}
			if !strings.Contains(first, pending.Interactions[0].ID) {
				t.Fatalf("initial response omitted pending interaction ID: %s", first)
			}
			wantStatus := http.StatusOK
			answers := `{"answers":{"source":{"answers":["PDF"]},"password":{"answers":["user_note: private-fixture"]}}}`
			switch action {
			case "stream":
				answers = `{"stream":true,` + strings.TrimPrefix(answers, "{")
			case "skip":
				answers = `{"answers":{}}`
				wantStatus = http.StatusNoContent
			}
			path := sessionQuestionPath + "/interactions/" + pending.Interactions[0].ID + "/respond"
			body = sessionInteractionHTTP(t, server, http.MethodPost, path, answers, wantStatus)
			switch action {
			case "stream":
				if !strings.Contains(body, "event: message_stop") || !strings.Contains(body, "Ready") {
					t.Fatalf("continuation SSE: %s", body)
				}
			case "json":
				var response agentSessionResponse
				if err := json.Unmarshal([]byte(body), &response); err != nil || response.Status != "completed" || len(response.Output) != 1 || len(response.Output[0].Content) != 1 || response.Output[0].Content[0].Text != "Ready" {
					t.Fatalf("continuation JSON: %s, %v", body, err)
				}
			}
			sessionInteractionHTTP(t, server, http.MethodPost, path, answers, http.StatusConflict)
			calls := engine.client.Calls()
			if action == "skip" {
				if len(calls) != 1 {
					t.Fatalf("skipping all questions dispatched %d turns", len(calls))
				}
				return
			}
			if len(calls) != 2 || calls[1].Request.ConversationKey != calls[0].Request.ConversationKey {
				t.Fatalf("detached continuation dispatched unexpected turns: %+v", calls)
			}
			prompt := sessionTurnText(calls[1].Request)
			if !strings.Contains(prompt, "PDF") || strings.Contains(prompt, "private-fixture") {
				t.Error("continuation prompt must include the selected document type and omit the secret answer")
			}
		})
	}
}

func TestAgentSessionRejectsUnsupportedInteractions(t *testing.T) {
	for _, tc := range []struct {
		name             string
		requestUserInput bool
		interaction      agentengine.InteractionKind
	}{
		{name: "legacy", interaction: agentengine.InteractionUserInput},
		{name: "permission", requestUserInput: true, interaction: agentengine.InteractionPermission},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
				event := sessionQuestionEvent()
				if tc.interaction == agentengine.InteractionPermission {
					event.Interaction = &agentengine.InteractionRequest{ID: "approval-1", Kind: tc.interaction}
				}
				_ = sink.Emit(ctx, event)
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded}
			}}
			handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
			body := map[string]any{"input": "Plan", "stream": true}
			if tc.requestUserInput {
				body["user_input"] = true
			}
			response := performAgentSessionRequest(t, handler, "Alpha", tc.name, body)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: error") || strings.Contains(response.Body.String(), "event: request_user_input") {
				t.Fatalf("response: HTTP %d %s", response.Code, response.Body.String())
			}
			if len(engine.client.Resolutions()) != 0 {
				t.Fatal("runtime interaction was resolved")
			}
		})
	}
}

func TestAgentSessionUserInputAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, bearer string
		noAuth       bool
		authorized   bool
	}{
		{name: "anonymous"},
		{name: "wrong token", bearer: "Bearer wrong-token"},
		{name: "server token", bearer: "Bearer test-server-token", authorized: true},
		{name: "local no auth", noAuth: true, authorized: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, &fakeSessionEngine{}, "")
			handler.serverNoAuth = tc.noAuth
			server := httptest.NewServer(handler.Routes())
			t.Cleanup(server.Close)
			for _, endpoint := range []struct {
				method, path, body string
				success            int
			}{
				{http.MethodPost, "/responses", `{"input":"hello"}`, http.StatusOK},
				{http.MethodPost, "/responses", `{"input":"hello","user_input":true}`, http.StatusOK},
				{http.MethodGet, "/interactions", "", http.StatusOK},
				{http.MethodPost, "/interactions/unknown/respond", `{"answers":{}}`, http.StatusNotFound},
			} {
				status, body := sessionFileHTTP(t, server, endpoint.method, sessionQuestionPath+endpoint.path, "application/json", strings.NewReader(endpoint.body), tc.bearer)
				want := endpoint.success
				if !tc.authorized && endpoint.body != `{"input":"hello"}` {
					want = http.StatusUnauthorized
				}
				if status != want {
					t.Errorf("%s %s %s: HTTP %d, want %d: %s", endpoint.method, endpoint.path, endpoint.body, status, want, body)
				}
			}
		})
	}
}

func TestAgentSessionDetachedAnswerRuntimeFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		dispatched bool
	}{{"before_dispatch", false}, {"after_dispatch", true}} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
				if sessionTurnText(request) == "Plan" {
					if err := sink.Emit(ctx, sessionDetachedQuestionEvent()); err != nil {
						t.Errorf("emit question: %v", err)
					}
				} else if attempts.Add(1) == 1 {
					return agentengine.TurnResult{
						Status: agentengine.TurnFailed, Dispatched: tc.dispatched,
						Error: &agentengine.TurnError{Code: agentengine.ErrorRuntimeAdapterUnavailable, Message: "runtime temporarily unavailable"},
					}
				}
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true, Output: "Ready"}
			}}
			server, _ := sessionOutputTestServer(t, engine, "")
			first := sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/responses", `{"input":"Plan","user_input":true}`, http.StatusOK)
			id := sessionResponseInteractionID(t, first)
			path := sessionQuestionPath + "/interactions/" + id + "/respond"
			const answers = `{"answers":{"source":{"answers":["PDF"]},"password":{"answers":[]}}}`
			sessionInteractionHTTP(t, server, http.MethodPost, path, answers, http.StatusServiceUnavailable)
			if !tc.dispatched {
				pending := sessionInteractionHTTP(t, server, http.MethodGet, sessionQuestionPath+"/interactions", "", http.StatusOK)
				if got := sessionResponseInteractionID(t, pending); got != id {
					t.Fatalf("pending interaction = %q, want %q", got, id)
				}
				sessionInteractionHTTP(t, server, http.MethodPost, path, answers, http.StatusOK)
			}
			sessionInteractionHTTP(t, server, http.MethodPost, path, answers, http.StatusConflict)
			delivered := 0
			for _, call := range engine.client.Calls() {
				if call.Result.Dispatched && strings.Contains(sessionTurnText(call.Request), "PDF") {
					delivered++
				}
			}
			if delivered != 1 {
				t.Fatalf("runtime accepted the answer %d times, want exactly one", delivered)
			}
		})
	}
}

func TestAgentSessionDetachedAnswerReportsReset(t *testing.T) {
	for _, mode := range []string{"json", "stream"} {
		t.Run(mode, func(t *testing.T) {
			engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
				if err := sink.Emit(ctx, sessionDetachedQuestionEvent()); err != nil {
					t.Errorf("emit question: %v", err)
				}
				return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true}
			}}
			engine.beforeRun = func(_ context.Context) {
				// Reset during the answer request while its HTTP client is connected.
				if calls := engine.client.Calls(); len(calls) == 1 {
					if err := engine.Conversations("agent-alpha").Reset(t.Context(), calls[0].Request.ConversationKey); err != nil {
						t.Errorf("reset conversation: %v", err)
					}
				}
			}
			server, _ := sessionOutputTestServer(t, engine, "")
			first := sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/responses", `{"input":"Plan","user_input":true}`, http.StatusOK)
			path := sessionQuestionPath + "/interactions/" + sessionResponseInteractionID(t, first) + "/respond"
			answers := fmt.Sprintf(`{"stream":%t,"answers":{"source":{"answers":["PDF"]},"password":{"answers":[]}}}`, mode == "stream")
			if mode == "stream" {
				body := sessionInteractionHTTP(t, server, http.MethodPost, path, answers, http.StatusOK)
				if !strings.Contains(body, "event: error") || !strings.Contains(body, "event: message_stop") {
					t.Fatalf("reset did not terminate the stream with an error: %s", body)
				}
			} else {
				body := sessionInteractionHTTP(t, server, http.MethodPost, path, answers, http.StatusServiceUnavailable)
				var response agentSessionErrorEnvelope
				if err := json.Unmarshal([]byte(body), &response); err != nil || response.Error.Code == "" {
					t.Fatalf("missing error after reset: %s, %v", body, err)
				}
			}
		})
	}
}

func sessionResponseInteractionID(t *testing.T, body string) string {
	t.Helper()
	var response agentSessionResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil || len(response.Interactions) != 1 || response.Interactions[0].InteractionID == "" {
		t.Fatalf("expected one interaction: %s, %v", body, err)
	}
	return response.Interactions[0].InteractionID
}

func TestAgentSessionDetachedAnswerReservesTurnBeforeCompetingQuestion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	continuationReady := make(chan struct{})
	admitContinuation := make(chan struct{})
	var attempts atomic.Int32
	engine := &fakeSessionEngine{beforeRun: func(ctx context.Context) {
		if attempts.Add(1) == 2 {
			close(continuationReady)
			select {
			case <-admitContinuation:
			case <-ctx.Done():
			}
		}
	}}
	engine.run = func(ctx context.Context, _ string, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		switch sessionTurnText(request) {
		case "Plan", "Other":
			if err := sink.Emit(ctx, sessionDetachedQuestionEvent()); err != nil {
				t.Errorf("emit question: %v", err)
			}
		default:
			if !strings.Contains(sessionTurnText(request), "PDF") {
				t.Errorf("continuation omitted the answer: %+v", request)
			}
		}
		return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Dispatched: true, Output: "Ready"}
	}
	handler, _, _, _ := newAgentSessionTestHandler(t, []agent.Agent{sessionCodexAgent("agent-alpha", "Alpha")}, engine, "")
	server := httptest.NewServer(handler.Routes())
	defer server.Close()
	first := sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/responses", `{"input":"Plan","user_input":true}`, http.StatusOK)
	id := sessionResponseInteractionID(t, first)
	const answers = `{"answers":{"source":{"answers":["PDF"]},"password":{"answers":[]}}}`
	answerPath := sessionQuestionPath + "/interactions/" + id + "/respond"
	answer := startSessionRequest(ctx, server.Config.Handler, answerPath, answers)
	select {
	case <-continuationReady:
	case <-ctx.Done():
		t.Fatal("submission did not attempt the continuation")
	}

	const newQuestion = `{"input":"Other","user_input":true}`
	sessionInteractionHTTP(t, server, http.MethodPost, answerPath, answers, http.StatusConflict)
	sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/responses", newQuestion, http.StatusConflict)
	close(admitContinuation)
	select {
	case response := <-answer:
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Ready") {
			t.Fatalf("answer returned HTTP %d: %s", response.Code, response.Body.String())
		}
	case <-ctx.Done():
		t.Fatal("answer did not complete")
	}
	next := sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/responses", newQuestion, http.StatusOK)
	nextID := sessionResponseInteractionID(t, next)
	if nextID == id {
		t.Fatal("competing turn reused the old question")
	}
	sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/interactions/"+nextID+"/respond", answers, http.StatusOK)
}

func startSessionRequest(ctx context.Context, router http.Handler, path, body string) <-chan *httptest.ResponseRecorder {
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		r := httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer test-server-token")
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, r)
		result <- rec
	}()
	return result
}

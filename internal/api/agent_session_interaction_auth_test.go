package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
)

func TestAgentSessionAnonymousCancelPreservesAuthenticatedQuestion(t *testing.T) {
	resume := make(chan struct{})
	engine := &fakeSessionEngine{run: func(ctx context.Context, _ string, _ agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
		if err := sink.Emit(ctx, sessionQuestionEvent()); err != nil {
			t.Errorf("emit native question: %v", err)
		}
		select {
		case <-ctx.Done():
			return agentengine.TurnResult{Status: agentengine.TurnCanceled}
		case <-resume:
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Answer received"}
		}
	}}
	server, _ := sessionOutputTestServer(t, engine, "")
	stream := openSessionQuestionStream(t, server)
	status, body := sessionFileHTTP(t, server, http.MethodPost, sessionQuestionPath+"/responses/cancel", "application/json", nil, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous cancel: HTTP %d %s", status, body)
	}
	sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/interactions/"+sessionQuestionID+"/respond",
		`{"answers":{"source":{"answers":["PDF"]}}}`, http.StatusNoContent)
	close(resume)
	rest, err := io.ReadAll(stream)
	if err != nil || !strings.Contains(string(rest), "Answer received") {
		t.Fatalf("continued turn: %q, %v", rest, err)
	}
}

func TestAgentSessionAnonymousTurnPreservesAuthenticatedDetachedQuestion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	beforeAdmission := make(chan struct{})
	admit := make(chan struct{})
	var first atomic.Bool
	engine := &fakeSessionEngine{
		beforeRun: func(runCtx context.Context) {
			if first.CompareAndSwap(false, true) {
				close(beforeAdmission)
				select {
				case <-admit:
				case <-runCtx.Done():
				}
			}
		},
		run: func(ctx context.Context, _ string, request agentengine.TurnRequest, sink agentengine.EventSink) agentengine.TurnResult {
			if sessionTurnText(request) == "Plan" {
				if err := sink.Emit(ctx, sessionDetachedQuestionEvent()); err != nil {
					t.Errorf("emit detached question: %v", err)
				}
			}
			return agentengine.TurnResult{Status: agentengine.TurnSucceeded, Output: "Ready"}
		},
	}
	server, _ := sessionOutputTestServer(t, engine, "")
	path := sessionQuestionPath + "/responses"
	waiting := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(`{"input":"anonymous"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.Config.Handler.ServeHTTP(rec, req)
		waiting <- rec
	}()
	select {
	case <-beforeAdmission:
	case <-ctx.Done():
		t.Fatal("anonymous request did not reach admission")
	}
	original := sessionInteractionHTTP(t, server, http.MethodPost, path, `{"input":"Plan","user_input":true}`, http.StatusOK)
	form := sessionResponseInteractionID(t, original)
	close(admit)
	select {
	case response := <-waiting:
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous request admitted after form creation: HTTP %d %s", response.Code, response.Body.String())
		}
	case <-ctx.Done():
		t.Fatal("anonymous request did not finish")
	}
	// Once the form is visible, even a streaming request receives HTTP 401 before SSE begins.
	status, body := sessionFileHTTP(t, server, http.MethodPost, path, "application/json", strings.NewReader(`{"input":"anonymous","stream":true}`), "")
	if status != http.StatusUnauthorized || strings.Contains(string(body), "message_start") {
		t.Fatalf("anonymous request against pending form: HTTP %d %s", status, body)
	}
	sessionInteractionHTTP(t, server, http.MethodPost, sessionQuestionPath+"/interactions/"+form+"/respond", `{"answers":{}}`, http.StatusNoContent)
	status, body = sessionFileHTTP(t, server, http.MethodPost, path, "application/json", strings.NewReader(`{"input":"anonymous"}`), "")
	if status != http.StatusOK || !strings.Contains(string(body), "Ready") {
		t.Fatalf("anonymous request after form resolved: HTTP %d %s", status, body)
	}
}

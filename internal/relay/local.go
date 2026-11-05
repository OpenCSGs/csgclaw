package relay

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"csgclaw/internal/apiclient"
)

// LocalExecutor uses the existing Session API -> Agent Engine -> Runtime path.
// Only this explicitly selected local agent is exposed; local API credentials
// are never sent to the relay.
type LocalExecutor struct {
	Client          *apiclient.Client
	AgentID         string
	Scope           string
	stopUnconfirmed atomic.Bool
}

func (e *LocalExecutor) Ready(ctx context.Context) bool {
	if e.stopUnconfirmed.Load() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	a, err := e.Client.GetAgent(ctx, e.AgentID)
	return err == nil && a.Status == "running" && !a.Runtime.SandboxEnabled && (a.Runtime.Name == "codex" || a.Runtime.Name == "dsh") && (a.Runtime.Availability == nil || a.Runtime.Availability.State == "ready" || a.Runtime.Availability.State == "not_applicable")
}

func (e *LocalExecutor) Execute(ctx context.Context, frame Frame) Frame {
	result := Frame{Type: "response.failed", ResponseID: frame.ResponseID, Sequence: 1}
	text, err := InputText(frame.Input)
	if err != nil {
		result.Error = &ProtocolError{Code: "invalid_request", Message: err.Error()}
		return result
	}
	// Hash the relay origin, published agent and session to avoid collisions with
	// interactive sessions and sessions belonging to other relay instances.
	session := fmt.Sprintf("relay-%x", sha256.Sum256([]byte(e.Scope+"\x00"+frame.SessionID)))
	var response Response
	path := "/api/v1/agents/" + url.PathEscape(e.AgentID) + "/sessions/" + session + "/responses"
	err = e.Client.DoJSON(ctx, http.MethodPost, path, map[string]any{"input": text, "stream": false}, &response)
	if ctx.Err() != nil {
		// Closing an HTTP request alone is not proof that runtime cleanup has
		// finished. The Session cancel endpoint waits for Agent Engine cleanup.
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if stopErr := e.Client.DoNoContent(stopCtx, http.MethodPost, path+"/cancel"); stopErr != nil {
			e.stopUnconfirmed.Store(true)
			result.Error = &ProtocolError{Code: "cancellation_unconfirmed", Message: "local execution stop could not be confirmed; executor is not ready"}
			return result
		}
		result.Type = "response.cancelled"
		return result
	}
	if err != nil {
		result.Error = &ProtocolError{Code: "execution_failed", Message: "local Session API execution failed"}
		// Translate only this locally generated diagnostic. Never relay arbitrary
		// upstream error text, which may contain credentials or private inputs.
		if strings.Contains(err.Error(), "this request contains Responses tool definitions or tool-use history") && strings.Contains(err.Error(), "native Responses tool support") {
			result.Error = &ProtocolError{Code: "model_protocol_unsupported", Message: "the published agent requires a provider/model with native Responses tool support"}
		}
		return result
	}
	if response.Status != "completed" {
		result.Error = &ProtocolError{Code: "execution_failed", Message: "local execution did not complete"}
		return result
	}
	result.Type = "response.completed"
	result.OutputText = response.Text()
	return result
}

func safeID(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/\\?#\x00\r\n")
}

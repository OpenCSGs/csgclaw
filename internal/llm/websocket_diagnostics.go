package llm

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"sync"

	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/diagnostics"
	"github.com/gorilla/websocket"
)

// Each response.create supplies its own native identity. Response IDs bind
// subsequent upstream frames; a long-lived websocket is never itself a turn.
type websocketDiagnostics struct {
	mu        sync.Mutex
	ctx       context.Context
	profile   agent.AgentProfile
	pending   []*websocketExchange
	responses map[string]*websocketExchange
}
type websocketExchange struct {
	id       string
	session  string
	turn     string
	recorder *diagnostics.Recorder
}

func newWebsocketDiagnostics(ctx context.Context, profile agent.AgentProfile) *websocketDiagnostics {
	return &websocketDiagnostics{ctx: ctx, profile: profile, responses: map[string]*websocketExchange{}}
}
func websocketNativeIdentity(payload []byte) (string, string) {
	var message map[string]json.RawMessage
	if json.Unmarshal(payload, &message) != nil {
		return "", ""
	}
	if response := message["response"]; len(response) > 0 {
		var nested map[string]json.RawMessage
		if json.Unmarshal(response, &nested) == nil {
			for key, value := range nested {
				message[key] = value
			}
		}
	}
	candidates := []json.RawMessage{message["turn_metadata"], message["metadata"]}
	var client map[string]json.RawMessage
	if json.Unmarshal(message["client_metadata"], &client) == nil {
		candidates = append(candidates, client["x-codex-turn-metadata"], client["turn_metadata"])
	}
	for _, candidate := range candidates {
		var encoded string
		if json.Unmarshal(candidate, &encoded) == nil {
			candidate = []byte(encoded)
		}
		var identity struct {
			Thread  string `json:"thread_id"`
			Session string `json:"session_id"`
			Turn    string `json:"turn_id"`
		}
		if json.Unmarshal(candidate, &identity) == nil {
			if identity.Thread == "" {
				identity.Thread = identity.Session
			}
			if identity.Thread != "" && identity.Turn != "" && len(identity.Thread) <= 256 && len(identity.Turn) <= 256 {
				return identity.Thread, identity.Turn
			}
		}
	}
	return "", ""
}
func (d *websocketDiagnostics) request(messageType int, original, rewritten []byte) {
	if messageType != websocket.TextMessage {
		return
	}
	var message struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(original, &message) != nil || message.Type != "response.create" {
		return
	}
	session, turn := websocketNativeIdentity(original)
	exchange := &websocketExchange{id: rand.Text(), session: session, turn: turn}
	if session != "" && turn != "" {
		ctx := diagnostics.WithNativeIdentity(d.ctx, session, turn)
		exchange.recorder = diagnostics.NewRecorder(ctx, d.profile.APIKey)
		exchange.recorder.Capture("model.runtime_request", map[string]any{"transport": "websocket", "request_id": exchange.id, "body": json.RawMessage(original)})
		exchange.recorder.Capture("model.upstream_request", map[string]any{"transport": "websocket", "request_id": exchange.id, "model": d.profile.ModelID, "body": json.RawMessage(rewritten)})
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.pending) >= 128 {
		d.pending = d.pending[1:]
	}
	d.pending = append(d.pending, exchange)
}
func (d *websocketDiagnostics) response(messageType int, payload []byte) {
	if messageType != websocket.TextMessage {
		return
	}
	var event struct {
		Type       string `json:"type"`
		ResponseID string `json:"response_id"`
		Response   struct {
			ID string `json:"id"`
		} `json:"response"`
	}
	if json.Unmarshal(payload, &event) != nil {
		return
	}
	id := event.ResponseID
	if id == "" {
		id = event.Response.ID
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	exchange := d.responses[id]
	if exchange == nil && event.Type == "response.created" {
		session, turn := websocketNativeIdentity(payload)
		index := -1
		if session != "" && turn != "" {
			for i, candidate := range d.pending {
				if candidate.session == session && candidate.turn == turn {
					index = i
					break
				}
			}
		} else if len(d.pending) == 1 {
			index = 0
		}
		if index >= 0 {
			exchange = d.pending[index]
			d.pending = append(d.pending[:index], d.pending[index+1:]...)
			if id != "" && len(d.responses) < 128 {
				d.responses[id] = exchange
			}
		} else if len(d.pending) > 1 {
			for _, candidate := range d.pending {
				candidate.recorder.Capture("collection.issue", map[string]string{"reason": "ambiguous_websocket_response_identity"})
			}
			d.pending = nil
		}
	}

	// Errors without a response ID can belong only to the first pending request.
	if exchange == nil && event.Type == "error" && len(d.pending) == 1 {
		exchange = d.pending[0]
		d.pending = nil
	}
	if exchange == nil || exchange.recorder == nil {
		return
	}
	value := map[string]any{"transport": "websocket", "request_id": exchange.id, "response_id": id, "body": json.RawMessage(payload)}
	exchange.recorder.Capture("model.upstream_response", value)
	exchange.recorder.Capture("model.runtime_response", value)
	if event.Type == "response.completed" || event.Type == "response.failed" || event.Type == "error" {
		exchange.recorder.Capture("model.body_end", map[string]any{"request_id": exchange.id, "complete": true, "stage": "websocket"})
		delete(d.responses, id)
	}
}

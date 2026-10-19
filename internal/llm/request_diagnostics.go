package llm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/diagnostics"
	"csgclaw/internal/version"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

type modelRequestContextKey struct{}
type modelRequestContext struct{ id, agent, provider, model, runtimeModel string }
type nativeRequestContextKey struct{}
type nativeRequestContext struct{ session, turn string }

func WithNativeRequestMetadata(ctx context.Context, session, turn string) context.Context {
	if len(session) > 256 || len(turn) > 256 {
		return ctx
	}
	return context.WithValue(ctx, nativeRequestContextKey{}, nativeRequestContext{session, turn})
}

func withModelRequestContext(ctx context.Context, agentID string, profile agent.AgentProfile, body []byte) context.Context {
	var input struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &input)
	provider := profile.ModelProviderID
	if provider == "" {
		provider = profile.Provider
	}
	metadata := modelRequestContext{rand.Text(), agentID, provider, profile.ModelID, input.Model}
	ctx = diagnostics.WithRequestModel(ctx, profile.ModelID)
	return context.WithValue(ctx, modelRequestContextKey{}, metadata)
}

// Keep only request shape and identities. Prompts, tool arguments, schemas and
// authentication headers must never enter the request summary.
func modelRequestLogAttrs(req *http.Request) []any {
	if req == nil {
		return nil
	}
	metadata, _ := req.Context().Value(modelRequestContextKey{}).(modelRequestContext)
	native, _ := req.Context().Value(nativeRequestContextKey{}).(nativeRequestContext)
	attrs := []any{"request_id", metadata.id, "agent_id", metadata.agent, "model_provider", metadata.provider, "model", metadata.model, "runtime_model", metadata.runtimeModel, "operation", req.URL.Path, "upstream_host", req.URL.Host, "request_bytes", req.ContentLength, "app_version", version.Version, "app_commit", version.Commit}
	if native.session != "" {
		attrs = append(attrs, "runtime_session_id", native.session, "runtime_turn_id", native.turn)
	}
	if record := diagnostics.Resolve(req.Context()); record != nil {
		snapshot := record.Snapshot()
		attrs = append(attrs, "diagnostic_id", snapshot.ID, "runtime", snapshot.Runtime, "turn_id", snapshot.TurnID)
	}
	if req.GetBody != nil {
		if body, err := req.GetBody(); err == nil {
			var input struct {
				Tools               []json.RawMessage `json:"tools"`
				MaxTokens           json.RawMessage   `json:"max_tokens"`
				MaxCompletionTokens json.RawMessage   `json:"max_completion_tokens"`
				MaxOutputTokens     json.RawMessage   `json:"max_output_tokens"`
				Stream              bool              `json:"stream"`
			}
			err = json.NewDecoder(io.LimitReader(body, 10<<20)).Decode(&input)
			body.Close()
			if err == nil {
				attrs = append(attrs, "tool_count", len(input.Tools), "stream", input.Stream)
				for _, limit := range []struct {
					name string
					raw  json.RawMessage
				}{{"max_tokens", input.MaxTokens}, {"max_completion_tokens", input.MaxCompletionTokens}, {"max_output_tokens", input.MaxOutputTokens}} {
					var value int64
					if len(limit.raw) > 0 && json.Unmarshal(limit.raw, &value) == nil {
						attrs = append(attrs, limit.name, value)
					}
				}
				if len(input.Tools) > 0 {
					encoded, _ := json.Marshal(input.Tools)
					digest := sha256.Sum256(encoded)
					attrs = append(attrs, "tools_sha256", hex.EncodeToString(digest[:]))
				}
			}
		}
	}
	return attrs
}

func logModelTransportFailure(req *http.Request, err error) {
	code := "connection_failed"
	var timeout net.Error
	switch {
	case errors.Is(err, context.Canceled):
		code = "request_cancelled"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout) && timeout.Timeout():
		code = "upstream_timeout"
	}
	attrs := append(modelRequestLogAttrs(req), "code", code)
	slog.ErrorContext(req.Context(), "model upstream transport failed", attrs...)
}

func logModelRequest(req *http.Request) {
	attrs := modelRequestLogAttrs(req)
	// Synthetic callers without an Agent identity are internal probes.
	metadata, _ := req.Context().Value(modelRequestContextKey{}).(modelRequestContext)
	if strings.TrimSpace(metadata.agent) != "" {
		slog.InfoContext(req.Context(), "model upstream request", attrs...)
	}
}

package llm

import (
	"bufio"
	"bytes"
	"context"
	agent "csgclaw/internal/agentengine/agents"
	"csgclaw/internal/modelpreset"
	"csgclaw/internal/modelprovider"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/translator"
	_ "github.com/router-for-me/CLIProxyAPI/v8/sdk/translator/builtin"
)

// forwardProtocol keeps the runtime's schema stable while using the service's
// declared wire protocol. Translators preserve tool call IDs and streamed deltas.
func (s *Service) forwardProtocol(ctx context.Context, profile agent.AgentProfile, payload map[string]any, source string) (*UpstreamResponse, error) {
	from := translator.Format(source)
	to, path := translator.FormatOpenAI, "/chat/completions"
	switch profile.APIProtocol {
	case modelpreset.Responses:
		to, path = translator.FormatOpenAIResponse, "/responses"
	case modelpreset.AnthropicMessages:
		to, path = translator.FormatClaude, "/messages"
	}
	if !translator.HasRequestTransformer(from, to) {
		return nil, &HTTPError{Status: 502, Message: "Unsupported model protocol conversion"}
	}
	original, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	stream := payloadBool(payload["stream"])
	// Claude translators aggregate SSE for non-streaming callers as well.
	encoded := translator.TranslateRequest(from, to, profile.ModelID, original, stream || to == translator.FormatClaude)
	if stream && to == translator.FormatOpenAI {
		var translated map[string]any
		if err := json.Unmarshal(encoded, &translated); err != nil {
			return nil, err
		}
		includeStreamUsage(translated)
		encoded, err = json.Marshal(translated)
		if err != nil {
			return nil, err
		}
	}
	base, key, err := s.agentProfileTarget(ctx, profile)
	if err != nil {
		return nil, err
	}
	c := modelprovider.ServiceConnection{BaseURL: base, APIKey: key, Protocol: profile.APIProtocol, Headers: profile.Headers}
	req, err := c.Request(ctx, http.MethodPost, path, encoded)
	if err != nil {
		return nil, err
	}
	resp, err := s.modelRequest(req)
	if err != nil {
		return nil, &HTTPError{Status: 502, Message: "Cannot reach the model service"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return compactUpstreamErrorStream(resp, OpenCSGBillingURL(profile))
	}
	header := make(http.Header)
	if !stream {
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if err != nil {
			return nil, err
		}
		var state any
		converted := translator.TranslateNonStream(ctx, to, from, profile.ModelID, original, encoded, body, &state)
		header.Set("Content-Type", "application/json")
		return &UpstreamResponse{StatusCode: resp.StatusCode, Header: header, Body: io.NopCloser(bytes.NewReader(converted))}, nil
	}
	header.Set("Content-Type", "text/event-stream")
	reader, writer := io.Pipe()
	// Closing the consumer cancels the upstream read, including while waiting for a chunk.
	streamCtx, cancel := context.WithCancel(ctx)
	go func() { <-streamCtx.Done(); resp.Body.Close() }()
	go func() {
		defer cancel()
		defer resp.Body.Close()
		var state any
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64<<10), maxResponsesSSEEventBytes)
		for scanner.Scan() {
			line := scanner.Bytes()
			// Builtin translators consume SSE data lines (including the data: prefix).
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			for _, chunk := range translator.TranslateStream(ctx, to, from, profile.ModelID, original, encoded, line, &state) {
				if len(chunk) == 0 {
					continue
				}
				if !bytes.HasPrefix(chunk, []byte("data:")) && !bytes.HasPrefix(chunk, []byte("event:")) {
					chunk = append(append([]byte("data: "), chunk...), '\n', '\n')
				}
				if _, err := writer.Write(chunk); err != nil {
					writer.CloseWithError(err)
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			writer.CloseWithError(fmt.Errorf("read model stream: %w", err))
			return
		}
		writer.Close()
	}()
	return &UpstreamResponse{StatusCode: resp.StatusCode, Header: header, Body: &protocolStream{ReadCloser: reader, cancel: cancel}}, nil
}

type protocolStream struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (s *protocolStream) Close() error { s.cancel(); return s.ReadCloser.Close() }

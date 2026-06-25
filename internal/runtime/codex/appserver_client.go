package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

type appServerClient struct {
	stdin  io.Writer
	logger *slog.Logger

	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int64
	pending map[int64]*appServerPendingRequest

	onNotification  func(appServerNotification)
	onServerRequest func(appServerServerRequest) (any, error)
}

type appServerPendingRequest struct {
	method string
	ch     chan appServerRPCResult
}

type appServerRPCResult struct {
	result json.RawMessage
	err    error
}

func newAppServerClient(stdin io.Writer, logger *slog.Logger) *appServerClient {
	return &appServerClient{
		stdin:   stdin,
		logger:  logger,
		pending: make(map[int64]*appServerPendingRequest),
	}
}

func (c *appServerClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c == nil || c.stdin == nil {
		return nil, fmt.Errorf("codex app-server client is not configured")
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return nil, fmt.Errorf("json-rpc method is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	c.mu.Lock()
	c.nextID++
	id := c.nextID
	pending := &appServerPendingRequest{
		method: method,
		ch:     make(chan appServerRPCResult, 1),
	}
	c.pending[id] = pending
	c.mu.Unlock()

	msg := map[string]any{
		"jsonrpc": appServerJSONRPCVersion,
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	if err := c.writeJSONLine(msg); err != nil {
		c.removePending(id)
		return nil, fmt.Errorf("write %s: %w", method, err)
	}

	select {
	case result := <-pending.ch:
		if result.err != nil {
			return nil, fmt.Errorf("%s: %w", method, result.err)
		}
		return result.result, nil
	case <-ctx.Done():
		c.removePending(id)
		return nil, ctx.Err()
	}
}

// notify sends a JSON-RPC notification (no id, no response expected) to the
// codex app-server. Used for the "initialized" notification that completes
// the app-server handshake.
func (c *appServerClient) notify(method string) {
	if c == nil || c.stdin == nil {
		return
	}
	method = strings.TrimSpace(method)
	if method == "" {
		return
	}
	msg := map[string]any{
		"jsonrpc": appServerJSONRPCVersion,
		"method":  method,
	}
	if err := c.writeJSONLine(msg); err != nil {
		c.logDebug("write codex app-server notification failed", "method", method, "error", err)
	}
}
func (c *appServerClient) handleLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	var msg appServerWireMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		c.logDebug("ignore malformed codex app-server json-rpc line", "error", err)
		return
	}
	c.handleMessage(msg)
}

func (c *appServerClient) handleMessage(msg appServerWireMessage) {
	hasID := len(msg.ID) > 0
	hasResult := len(msg.Result) > 0
	hasError := len(msg.Error) > 0
	method := strings.TrimSpace(msg.Method)
	switch {
	case hasID && (hasResult || hasError):
		c.handleResponse(msg)
	case hasID && method != "":
		c.handleServerRequest(msg)
	case !hasID && method != "":
		c.handleNotification(msg)
	default:
		c.logDebug("ignore unrecognized codex app-server json-rpc message")
	}
}

func (c *appServerClient) handleResponse(msg appServerWireMessage) {
	id, err := decodeAppServerNumericID(msg.ID)
	if err != nil {
		c.logDebug("ignore codex app-server response with invalid id", "error", err)
		return
	}
	pending := c.removePending(id)
	if pending == nil {
		c.logDebug("ignore codex app-server response with unknown id", "id", id)
		return
	}

	if len(msg.Error) > 0 {
		var rpcErr appServerRPCError
		if err := json.Unmarshal(msg.Error, &rpcErr); err != nil {
			pending.ch <- appServerRPCResult{err: fmt.Errorf("decode JSON-RPC error: %w", err)}
			return
		}
		pending.ch <- appServerRPCResult{err: rpcErr}
		return
	}
	pending.ch <- appServerRPCResult{result: msg.Result}
}

func (c *appServerClient) handleServerRequest(msg appServerWireMessage) {
	req := appServerServerRequest{
		ID:     cloneRawMessage(msg.ID),
		Method: strings.TrimSpace(msg.Method),
		Params: cloneRawMessage(msg.Params),
	}
	handler := c.onServerRequest
	if handler == nil {
		_ = c.respondError(req.ID, -32601, fmt.Sprintf("unhandled server request: %s", req.Method))
		return
	}
	result, err := handler(req)
	if err != nil {
		_ = c.respondError(req.ID, -32603, err.Error())
		return
	}
	if err := c.respond(req.ID, result); err != nil {
		c.logDebug("write codex app-server server-request response failed", "method", req.Method, "error", err)
	}
}

func (c *appServerClient) handleNotification(msg appServerWireMessage) {
	handler := c.onNotification
	if handler == nil {
		return
	}
	handler(appServerNotification{
		Method: strings.TrimSpace(msg.Method),
		Params: cloneRawMessage(msg.Params),
	})
}

func (c *appServerClient) respond(id json.RawMessage, result any) error {
	if len(id) == 0 {
		return fmt.Errorf("json-rpc response id is required")
	}
	msg := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{
		JSONRPC: appServerJSONRPCVersion,
		ID:      id,
		Result:  result,
	}
	return c.writeJSONLine(msg)
}

func (c *appServerClient) respondError(id json.RawMessage, code int, message string) error {
	if len(id) == 0 {
		return fmt.Errorf("json-rpc response id is required")
	}
	msg := struct {
		JSONRPC string            `json:"jsonrpc"`
		ID      json.RawMessage   `json:"id"`
		Error   appServerRPCError `json:"error"`
	}{
		JSONRPC: appServerJSONRPCVersion,
		ID:      id,
		Error: appServerRPCError{
			Code:    code,
			Message: message,
		},
	}
	return c.writeJSONLine(msg)
}

func (c *appServerClient) writeJSONLine(msg any) error {
	if c == nil || c.stdin == nil {
		return fmt.Errorf("codex app-server client is not configured")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.stdin.Write(data)
	return err
}

func (c *appServerClient) closeAllPending(err error) {
	if err == nil {
		err = fmt.Errorf("codex app-server client closed")
	}
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[int64]*appServerPendingRequest)
	c.mu.Unlock()
	for _, req := range pending {
		req.ch <- appServerRPCResult{err: err}
	}
}

func (c *appServerClient) removePending(id int64) *appServerPendingRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := c.pending[id]
	delete(c.pending, id)
	return pending
}

func (c *appServerClient) logDebug(msg string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Debug(msg, args...)
	}
}

func decodeAppServerNumericID(raw json.RawMessage) (int64, error) {
	var id int64
	if err := json.Unmarshal(raw, &id); err == nil {
		return id, nil
	}
	var floatID float64
	if err := json.Unmarshal(raw, &floatID); err != nil {
		return 0, err
	}
	if floatID != float64(int64(floatID)) {
		return 0, fmt.Errorf("non-integer id %s", string(raw))
	}
	return int64(floatID), nil
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

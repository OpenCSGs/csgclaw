package codex

import (
	"encoding/json"
	"fmt"
	"strings"
)

const appServerJSONRPCVersion = "2.0"

type appServerNotification struct {
	Method string
	Params json.RawMessage
}

type appServerServerRequest struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

type appServerRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e appServerRPCError) Error() string {
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = "JSON-RPC error"
	}
	if len(e.Data) == 0 || string(e.Data) == "null" {
		return fmt.Sprintf("%s (code=%d)", message, e.Code)
	}
	return fmt.Sprintf("%s (code=%d, data=%s)", message, e.Code, strings.TrimSpace(string(e.Data)))
}

type appServerWireMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

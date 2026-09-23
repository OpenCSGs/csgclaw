package apps

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Refresh only an explicit authentication rejection. Transport errors, scope
// failures and arbitrary business errors must never replay a write operation.
func callAppTool(ctx context.Context, conn *connection, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	for attempt := 0; attempt < 2; attempt++ {
		requestContext := ctx
		token := ""
		if conn.tokens != nil {
			var err error
			token, err = conn.tokens.Token(ctx)
			if err != nil {
				return nil, connectionError("app_feishu_token_failed", "Cannot obtain a Feishu application token. Check the connector App ID/App Secret, application status and Feishu connectivity.", 0, true)
			}
			requestContext = context.WithValue(ctx, feishuCallTokenKey{}, token)
		}
		result, err := conn.session.CallTool(requestContext, params)
		if conn.tokens == nil || !(feishuTokenRejected(result) || feishuRPCTokenRejected(err)) {
			return result, err
		}
		conn.tokens.Invalidate(token)
		if attempt == 1 {
			return result, err
		}
	}
	panic("unreachable")
}

func feishuTokenRejected(result *mcp.CallToolResult) bool {
	if result == nil || !result.IsError {
		return false
	}
	check := func(value any) bool {
		data, err := json.Marshal(value)
		if err != nil {
			return false
		}
		var envelope struct {
			Code  string `json:"code"`
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &envelope) != nil {
			return false
		}
		return envelope.Code == "lark_token_invalid" || envelope.Error.Code == "lark_token_invalid"
	}
	if check(result.StructuredContent) {
		return true
	}
	for _, block := range result.Content {
		if text, ok := block.(*mcp.TextContent); ok {
			var value any
			if json.Unmarshal([]byte(text.Text), &value) == nil && check(value) {
				return true
			}
		}
	}
	return false
}

func feishuRPCTokenRejected(err error) bool {
	var rpcErr *jsonrpc.Error
	if !errors.As(err, &rpcErr) {
		return false
	}
	var data struct {
		Code string `json:"code"`
	}
	return json.Unmarshal(rpcErr.Data, &data) == nil && data.Code == "lark_token_invalid"
}

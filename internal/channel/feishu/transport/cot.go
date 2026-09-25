package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	channel "csgclaw/internal/channel"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type COTRef struct {
	COTID     string
	MessageID string
}
type COTCreateRequest struct {
	ChatID          string
	OriginMessageID string
}
type COTUpdateRequest struct {
	Ref    COTRef
	Events []channel.COTEvent
}
type COTCompleteRequest struct {
	Ref    COTRef
	Reason string
}

// COTAdapter uses the native process-message API on the existing app identity.
type COTAdapter interface {
	CreateCOT(context.Context, COTCreateRequest) (COTRef, error)
	UpdateCOT(context.Context, COTUpdateRequest) error
	CompleteCOT(context.Context, COTCompleteRequest) error
}

func (a *adapter) CreateCOT(ctx context.Context, r COTCreateRequest) (COTRef, error) {
	if err := a.ready(ctx); err != nil {
		return COTRef{}, err
	}
	c, ok := a.oapi.(COTAdapter)
	if !ok {
		return COTRef{}, ErrUnsupportedEvent
	}
	return c.CreateCOT(ctx, r)
}
func (a *adapter) UpdateCOT(ctx context.Context, r COTUpdateRequest) error {
	if err := a.ready(ctx); err != nil {
		return err
	}
	c, ok := a.oapi.(COTAdapter)
	if !ok {
		return ErrUnsupportedEvent
	}
	return c.UpdateCOT(ctx, r)
}
func (a *adapter) CompleteCOT(ctx context.Context, r COTCompleteRequest) error {
	if err := a.ready(ctx); err != nil {
		return err
	}
	c, ok := a.oapi.(COTAdapter)
	if !ok {
		return ErrUnsupportedEvent
	}
	return c.CompleteCOT(ctx, r)
}
func (o *directOutbound) cotAPI() (COTAdapter, error) {
	a, ok := o.api.(COTAdapter)
	if !ok {
		return nil, ErrUnsupportedEvent
	}
	return a, nil
}
func (o *directOutbound) CreateCOT(ctx context.Context, r COTCreateRequest) (COTRef, error) {
	a, e := o.cotAPI()
	if e != nil {
		return COTRef{}, e
	}
	return a.CreateCOT(ctx, r)
}
func (o *directOutbound) UpdateCOT(ctx context.Context, r COTUpdateRequest) error {
	a, e := o.cotAPI()
	if e != nil {
		return e
	}
	return a.UpdateCOT(ctx, r)
}
func (o *directOutbound) CompleteCOT(ctx context.Context, r COTCompleteRequest) error {
	a, e := o.cotAPI()
	if e != nil {
		return e
	}
	return a.CompleteCOT(ctx, r)
}
func (a *sdkLarkOpenAPI) CreateCOT(ctx context.Context, r COTCreateRequest) (COTRef, error) {
	if r.ChatID == "" {
		return COTRef{}, fmt.Errorf("COT chat ID is required")
	}
	q := larkcore.QueryParams{}
	q.Set("receive_id_type", "chat_id")
	body := map[string]any{"receive_id": r.ChatID}
	if r.OriginMessageID != "" {
		body["origin_message_id"] = r.OriginMessageID
	}
	data, err := a.cotRequest(ctx, "create COT", http.MethodPost, "/open-apis/im/v1/message_cot", q, body)
	if err != nil {
		return COTRef{}, err
	}
	var out struct {
		COTID     string `json:"cot_id"`
		MessageID string `json:"message_id"`
	}
	if json.Unmarshal(data, &out) != nil || out.COTID == "" || out.MessageID == "" {
		return COTRef{}, fmt.Errorf("COT response has no message identifiers")
	}
	return COTRef{out.COTID, out.MessageID}, nil
}
func (a *sdkLarkOpenAPI) UpdateCOT(ctx context.Context, r COTUpdateRequest) error {
	if r.Ref.COTID == "" || r.Ref.MessageID == "" {
		return fmt.Errorf("COT identifiers are required")
	}
	if len(r.Events) == 0 {
		return nil
	}
	_, err := a.cotRequest(ctx, "update COT", http.MethodPut, "/open-apis/im/v1/message_cot", nil, map[string]any{"cot_id": r.Ref.COTID, "message_id": r.Ref.MessageID, "events": r.Events})
	return err
}
func (a *sdkLarkOpenAPI) CompleteCOT(ctx context.Context, r COTCompleteRequest) error {
	if r.Ref.COTID == "" || r.Ref.MessageID == "" {
		return fmt.Errorf("COT identifiers are required")
	}
	q := larkcore.QueryParams{}
	q.Set("message_id", r.Ref.MessageID)
	q.Set("reason", r.Reason)
	_, err := a.cotRequest(ctx, "complete COT", http.MethodPost, "/open-apis/im/v1/message_cot/complete/"+url.PathEscape(r.Ref.COTID), q, nil)
	// The native stop button can finish the COT before backend cancellation finishes.
	// Completing that same terminal COT is an idempotent success.
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus == http.StatusOK && apiErr.Code == 10001 && strings.HasSuffix(apiErr.Message, "ext=CompleteCOT: already in terminal status") {
		return nil
	}
	return err
}
func (a *sdkLarkOpenAPI) cotRequest(ctx context.Context, operation, method, path string, q larkcore.QueryParams, body any) (json.RawMessage, error) {
	token, err := a.token(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(ctx, &larkcore.ApiReq{HttpMethod: method, ApiPath: path, QueryParams: q, Body: body, SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant}}, larkcore.WithTenantAccessToken(token))
	if err != nil {
		return nil, &APIError{Operation: operation, cause: err}
	}
	var out struct {
		Code    int             `json:"code"`
		Message string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(resp.RawBody, &out) != nil {
		return nil, &APIError{Operation: operation, HTTPStatus: resp.StatusCode, Message: "invalid response"}
	}
	a.invalidateRejectedToken(token, out.Code)
	if out.Code != 0 || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Operation: operation, Code: out.Code, HTTPStatus: resp.StatusCode, Message: sanitizeAPIMessage(out.Message)}
	}
	return out.Data, nil
}

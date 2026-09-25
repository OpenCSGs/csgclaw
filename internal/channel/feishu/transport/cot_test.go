package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	channel "csgclaw/internal/channel"

	lark "github.com/larksuite/oapi-sdk-go/v3"
)

func TestNativeCOTUsesAppIdentityAndSeparateEndpoints(t *testing.T) {
	var methods, paths []string
	client := lark.NewClient("app", "secret", lark.WithEnableTokenCache(false), lark.WithHttpClient(&singleAttemptHTTPClient{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		methods = append(methods, req.Method)
		paths = append(paths, req.URL.Path)
		if req.Header.Get("Authorization") != "Bearer tenant-token" {
			t.Error("missing tenant identity")
		}
		if len(methods) == 1 {
			var body map[string]string
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["origin_message_id"] != "origin" || body["receive_id"] != "chat" || req.URL.Query().Get("receive_id_type") != "chat_id" {
				t.Fatalf("create=%+v", body)
			}
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"cot_id":"cot","message_id":"message"}}`))}, nil
	})}}))
	outbound := newDirectOutbound(client, tenantTokenSourceFunc(func(context.Context) (string, error) { return "tenant-token", nil }))
	ref, err := outbound.CreateCOT(context.Background(), COTCreateRequest{ChatID: "chat", OriginMessageID: "origin"})
	if err != nil {
		t.Fatal(err)
	}
	if err = outbound.UpdateCOT(context.Background(), COTUpdateRequest{Ref: ref, Events: []channel.COTEvent{{EventType: "RUN_STARTED", Content: `{}`, Timestamp: 1}}}); err != nil {
		t.Fatal(err)
	}
	if err = outbound.CompleteCOT(context.Background(), COTCompleteRequest{Ref: ref, Reason: "done"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, ",") != "POST,PUT,POST" || paths[2] != "/open-apis/im/v1/message_cot/complete/cot" {
		t.Fatalf("requests=%v %v", methods, paths)
	}
}

func TestCOTCompletionPreservesSanitizedAPIError(t *testing.T) {
	client := lark.NewClient("app", "secret", lark.WithEnableTokenCache(false), lark.WithHttpClient(&singleAttemptHTTPClient{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":10001,"msg":"invalid completion reason"}`))}, nil
	})}}))
	outbound := newDirectOutbound(client, tenantTokenSourceFunc(func(context.Context) (string, error) { return "tenant-token", nil }))
	err := outbound.CompleteCOT(context.Background(), COTCompleteRequest{Ref: COTRef{COTID: "cot", MessageID: "message"}, Reason: "error"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != 10001 || apiErr.Operation != "complete COT" || apiErr.Message != "invalid completion reason" {
		t.Fatalf("error=%v", err)
	}
}

func TestCOTCompletionAcceptsOnlyAlreadyTerminalResponse(t *testing.T) {
	for _, test := range []struct {
		name         string
		code, status int
		message      string
		success      bool
	}{
		{"already terminal", 10001, 200, "Your request contains an invalid request parameter, ext=CompleteCOT: already in terminal status", true},
		{"other parameter", 10001, 200, "invalid completion reason", false},
		{"other code", 10002, 200, "ext=CompleteCOT: already in terminal status", false},
		{"http failure", 10001, 500, "ext=CompleteCOT: already in terminal status", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := lark.NewClient("app", "secret", lark.WithEnableTokenCache(false), lark.WithHttpClient(&singleAttemptHTTPClient{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body, _ := json.Marshal(map[string]any{"code": test.code, "msg": test.message})
				return &http.Response{StatusCode: test.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}}))
			outbound := newDirectOutbound(client, tenantTokenSourceFunc(func(context.Context) (string, error) { return "tenant-token", nil }))
			err := outbound.CompleteCOT(context.Background(), COTCompleteRequest{Ref: COTRef{COTID: "cot", MessageID: "message"}, Reason: "error"})
			if (err == nil) != test.success {
				t.Fatalf("success=%v error=%v", test.success, err)
			}
			if test.success {
				_, err = outbound.CreateCOT(context.Background(), COTCreateRequest{ChatID: "chat"})
				if err == nil {
					t.Fatal("creation error was suppressed")
				}
			}
		})
	}
}

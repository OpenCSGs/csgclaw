package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestDirectOutboundReplyErrorHasOneCallAndNoFallbackOrChunk(t *testing.T) {
	t.Parallel()
	replyErr := errors.New("reply target was withdrawn")
	api := &fakeLarkOpenAPI{replyErr: replyErr}
	outbound := newDirectOutboundWithAPI(api)
	text := strings.Repeat("long answer ", 100)
	key := "delivery/with/a/very/long/raw/id/that/must/not/be/sent/to/feishu"

	_, err := outbound.SendCard(context.Background(), SendCardRequest{
		ChatID: "chat-1", Card: map[string]any{"text": text}, IdempotencyKey: key,
		ReplyTo: "message-1", ReplyInThread: true, ThreadID: "thread-1",
	})
	if !errors.Is(err, replyErr) {
		t.Fatalf("SendText() error = %v, want reply error", err)
	}
	if api.replyCalls != 1 || api.createCalls != 0 {
		t.Fatalf("OpenAPI calls: reply=%d create=%d, want one reply and no fallback", api.replyCalls, api.createCalls)
	}
	if api.replyReq == nil || api.replyReq.Body == nil || api.replyReq.Body.Content == nil || api.replyReq.Body.Uuid == nil || api.replyReq.Body.ReplyInThread == nil || !*api.replyReq.Body.ReplyInThread || api.replyReq.MessageID != "message-1" {
		t.Fatalf("reply request = %#v", api.replyReq)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(*api.replyReq.Body.Content), &content); err != nil || content["text"] != text {
		t.Fatalf("reply content was changed or chunked: size=%d error=%v", len(content["text"]), err)
	}
	if *api.replyReq.Body.Uuid != feishuMessageUUID(key) || strings.Contains(*api.replyReq.Body.Uuid, key) {
		t.Fatalf("reply UUID = %q", *api.replyReq.Body.Uuid)
	}
}

func TestDirectOutboundCreateCardUsesStableUUIDAndExactJSON(t *testing.T) {
	t.Parallel()
	api := &fakeLarkOpenAPI{}
	outbound := newDirectOutboundWithAPI(api)
	card := map[string]any{
		"schema": "2.0",
		"body":   map[string]any{"elements": []any{map[string]any{"tag": "markdown", "content": "hello"}}},
	}
	key := strings.Repeat("durable-intent-id/", 20)

	result, err := outbound.SendCard(context.Background(), SendCardRequest{
		ChatID: " chat-1 ", Card: card, IdempotencyKey: key,
	})
	if err != nil || result.MessageID != "created-message" {
		t.Fatalf("SendCard() = %+v, %v", result, err)
	}
	if api.createCalls != 1 || api.replyCalls != 0 || api.createReq == nil || api.createReq.Body == nil || api.createReq.ReceiveIDType != "chat_id" {
		t.Fatalf("OpenAPI calls: create=%d reply=%d request=%#v", api.createCalls, api.replyCalls, api.createReq)
	}
	body := api.createReq.Body
	if body.ReceiveId == nil || *body.ReceiveId != "chat-1" || body.MsgType == nil || *body.MsgType != "interactive" || body.Content == nil || body.Uuid == nil {
		t.Fatalf("create body = %#v", body)
	}
	var gotCard map[string]any
	if err := json.Unmarshal([]byte(*body.Content), &gotCard); err != nil || !reflect.DeepEqual(gotCard, card) {
		t.Fatalf("create card = %#v, error=%v", gotCard, err)
	}
	uuid := *body.Uuid
	if uuid != feishuMessageUUID(key) || uuid != feishuMessageUUID(" "+key+" ") {
		t.Fatalf("UUID is not stable: %q", uuid)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(uuid) {
		t.Fatalf("UUID is not RFC 4122 version 8: %q", uuid)
	}
	if strings.Contains(uuid, key) || uuid == key || feishuMessageUUID(key+"different") == uuid {
		t.Fatalf("UUID leaked or failed to distinguish the raw key: %q", uuid)
	}
}

func TestDirectOutboundUploadsImageThenSendsMessage(t *testing.T) {
	t.Parallel()
	api := &fakeLarkOpenAPI{}
	outbound := newDirectOutboundWithAPI(api)
	key := "delivery-image-1"

	upload, err := outbound.UploadImage(context.Background(), UploadImageRequest{
		MediaType: "image/png", SizeBytes: 3, Content: strings.NewReader("png"),
	})
	if err != nil || upload.Key != "img-test" {
		t.Fatalf("UploadImage() = %+v, %v", upload, err)
	}
	if api.createImageCalls != 1 || api.createCalls != 0 || api.replyCalls != 0 || api.createImageReq == nil || api.createImageReq.Body == nil {
		t.Fatalf("OpenAPI calls after upload: image=%d create=%d reply=%d request=%#v", api.createImageCalls, api.createCalls, api.replyCalls, api.createImageReq)
	}
	result, err := outbound.SendImage(context.Background(), SendImageRequest{
		ChatID: " chat-1 ", ImageKey: upload.Key, IdempotencyKey: key,
	})
	if err != nil || result.MessageID != "created-message" {
		t.Fatalf("SendImage() = %+v, %v", result, err)
	}
	if api.createImageCalls != 1 || api.createCalls != 1 || api.replyCalls != 0 || api.createImageReq == nil || api.createImageReq.Body == nil {
		t.Fatalf("OpenAPI calls: upload image=%d create=%d reply=%d request=%#v", api.createImageCalls, api.createCalls, api.replyCalls, api.createImageReq)
	}
	if api.createImageReq.Body.ImageType == nil || *api.createImageReq.Body.ImageType != "message" {
		t.Fatalf("image upload body = %#v", api.createImageReq.Body)
	}
	uploaded, readErr := io.ReadAll(api.createImageReq.Body.Image)
	if readErr != nil || string(uploaded) != "png" {
		t.Fatalf("uploaded image = %q, error=%v", uploaded, readErr)
	}
	body := api.createReq.Body
	if body.MsgType == nil || *body.MsgType != larkim.MsgTypeImage || body.Content == nil || body.Uuid == nil || *body.Uuid != feishuMessageUUID(key) {
		t.Fatalf("image message body = %#v", body)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(*body.Content), &content); err != nil || content["image_key"] != "img-test" {
		t.Fatalf("image message content = %#v, error=%v", content, err)
	}
}

func TestDirectOutboundRejectsUnsupportedImageMediaTypeBeforeUpload(t *testing.T) {
	t.Parallel()
	api := &fakeLarkOpenAPI{}
	outbound := newDirectOutboundWithAPI(api)

	_, err := outbound.UploadImage(context.Background(), UploadImageRequest{
		MediaType: "image/svg+xml", SizeBytes: 3, Content: strings.NewReader("svg"),
	})
	if err == nil || api.createImageCalls != 0 || api.createCalls != 0 || api.replyCalls != 0 {
		t.Fatalf("SendImage() error=%v upload=%d create=%d reply=%d", err, api.createImageCalls, api.createCalls, api.replyCalls)
	}
}

func TestDirectOutboundRejectsEmptyUploadsBeforeOpenAPI(t *testing.T) {
	t.Parallel()
	api := &fakeLarkOpenAPI{}
	outbound := newDirectOutboundWithAPI(api)

	_, imageErr := outbound.UploadImage(context.Background(), UploadImageRequest{
		MediaType: "image/png", Content: strings.NewReader(""),
	})
	_, fileErr := outbound.UploadFile(context.Background(), UploadFileRequest{
		Name: "empty.txt", Content: strings.NewReader(""),
	})
	if imageErr == nil || fileErr == nil || api.createImageCalls != 0 || api.createFileCalls != 0 {
		t.Fatalf("image error=%v file error=%v image uploads=%d file uploads=%d", imageErr, fileErr, api.createImageCalls, api.createFileCalls)
	}
	if SupportsImageUpload("image/png", 0) {
		t.Fatal("zero-byte image was considered uploadable")
	}
}

func TestDirectOutboundUploadsFileThenRepliesWithMessage(t *testing.T) {
	t.Parallel()
	api := &fakeLarkOpenAPI{}
	outbound := newDirectOutboundWithAPI(api)
	key := "delivery-file-1"

	upload, err := outbound.UploadFile(context.Background(), UploadFileRequest{
		Name: " report.pdf ", SizeBytes: 3, Content: strings.NewReader("pdf"),
	})
	if err != nil || upload.Key != "file-test" {
		t.Fatalf("UploadFile() = %+v, %v", upload, err)
	}
	if api.createFileCalls != 1 || api.replyCalls != 0 || api.createCalls != 0 || api.createFileReq == nil || api.createFileReq.Body == nil {
		t.Fatalf("OpenAPI calls after upload: file=%d reply=%d create=%d request=%#v", api.createFileCalls, api.replyCalls, api.createCalls, api.createFileReq)
	}
	result, err := outbound.SendFile(context.Background(), SendFileRequest{
		ChatID: "chat-1", FileKey: upload.Key, IdempotencyKey: key,
		ReplyTo: "message-root", ReplyInThread: true, ThreadID: "thread-1",
	})
	if err != nil || result.MessageID != "reply-message" {
		t.Fatalf("SendFile() = %+v, %v", result, err)
	}
	if api.createFileCalls != 1 || api.replyCalls != 1 || api.createCalls != 0 || api.createFileReq == nil || api.createFileReq.Body == nil {
		t.Fatalf("OpenAPI calls: upload file=%d reply=%d create=%d request=%#v", api.createFileCalls, api.replyCalls, api.createCalls, api.createFileReq)
	}
	if api.createFileReq.Body.FileType == nil || *api.createFileReq.Body.FileType != "stream" ||
		api.createFileReq.Body.FileName == nil || *api.createFileReq.Body.FileName != "report.pdf" {
		t.Fatalf("file upload body = %#v", api.createFileReq.Body)
	}
	uploaded, readErr := io.ReadAll(api.createFileReq.Body.File)
	if readErr != nil || string(uploaded) != "pdf" {
		t.Fatalf("uploaded file = %q, error=%v", uploaded, readErr)
	}
	body := api.replyReq.Body
	if api.replyReq.MessageID != "message-root" || body.MsgType == nil || *body.MsgType != larkim.MsgTypeFile ||
		body.Content == nil || body.Uuid == nil || *body.Uuid != feishuMessageUUID(key) ||
		body.ReplyInThread == nil || !*body.ReplyInThread {
		t.Fatalf("file reply body = %#v request=%#v", body, api.replyReq)
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(*body.Content), &content); err != nil || content["file_key"] != "file-test" {
		t.Fatalf("file message content = %#v, error=%v", content, err)
	}
}

func TestDirectOutboundRejectsOversizedFileBeforeUpload(t *testing.T) {
	t.Parallel()
	api := &fakeLarkOpenAPI{}
	outbound := newDirectOutboundWithAPI(api)

	_, err := outbound.UploadFile(context.Background(), UploadFileRequest{
		Name: "archive.zip", SizeBytes: FileUploadLimitBytes + 1, Content: strings.NewReader("zip"),
	})
	if !errors.Is(err, ErrPayloadTooLarge) || api.createFileCalls != 0 {
		t.Fatalf("UploadFile() error=%v upload calls=%d", err, api.createFileCalls)
	}
}

func TestDirectOutboundPatchAndReactionsEachCallOnce(t *testing.T) {
	t.Parallel()
	api := &fakeLarkOpenAPI{}
	outbound := newDirectOutboundWithAPI(api)
	card := map[string]any{"schema": "2.0", "body": map[string]any{"elements": []any{}}}

	if err := outbound.UpdateCard(context.Background(), UpdateCardRequest{MessageID: " card-message ", Card: card}); err != nil {
		t.Fatalf("UpdateCard() error = %v", err)
	}
	if api.patchCalls != 1 || api.patchReq == nil || api.patchReq.MessageID != "card-message" || api.patchReq.Body == nil || api.patchReq.Body.Content == nil {
		t.Fatalf("patch calls=%d request=%#v", api.patchCalls, api.patchReq)
	}
	var gotCard map[string]any
	if err := json.Unmarshal([]byte(*api.patchReq.Body.Content), &gotCard); err != nil || !reflect.DeepEqual(gotCard, card) {
		t.Fatalf("patch card = %#v, error=%v", gotCard, err)
	}

	reaction, err := outbound.AddReaction(context.Background(), AddReactionRequest{MessageID: " message-1 ", EmojiType: " Pin "})
	if err != nil || reaction.ReactionID != "reaction-1" {
		t.Fatalf("AddReaction() = %+v, %v", reaction, err)
	}
	if api.createReactionCalls != 1 || api.createReactionReq == nil || api.createReactionReq.MessageID != "message-1" || api.createReactionReq.Body == nil || api.createReactionReq.Body.ReactionType == nil || api.createReactionReq.Body.ReactionType.EmojiType == nil || *api.createReactionReq.Body.ReactionType.EmojiType != "Pin" {
		t.Fatalf("create reaction calls=%d request=%#v", api.createReactionCalls, api.createReactionReq)
	}
	if err := outbound.DeleteReaction(context.Background(), DeleteReactionRequest{MessageID: " message-1 ", ReactionID: " reaction-1 "}); err != nil {
		t.Fatalf("DeleteReaction() error = %v", err)
	}
	if api.deleteReactionCalls != 1 || api.deleteReactionReq == nil || api.deleteReactionReq.MessageID != "message-1" || api.deleteReactionReq.ReactionID != "reaction-1" {
		t.Fatalf("delete reaction calls=%d request=%#v", api.deleteReactionCalls, api.deleteReactionReq)
	}
}

func TestDirectOutboundRejectsOversizeWireBodiesWithoutCallingAPI(t *testing.T) {
	t.Parallel()
	t.Run("card create", func(t *testing.T) {
		api := &fakeLarkOpenAPI{}
		outbound := newDirectOutboundWithAPI(api)
		_, err := outbound.SendCard(context.Background(), SendCardRequest{
			ChatID: "chat-1", IdempotencyKey: "oversize-card", Card: map[string]any{"content": strings.Repeat("\n", 20<<10)},
		})
		if !errors.Is(err, ErrPayloadTooLarge) || IsRetryable(err) || api.createCalls != 0 || api.replyCalls != 0 {
			t.Fatalf("SendCard() error=%v create=%d reply=%d", err, api.createCalls, api.replyCalls)
		}
	})

	t.Run("card patch", func(t *testing.T) {
		api := &fakeLarkOpenAPI{}
		outbound := newDirectOutboundWithAPI(api)
		err := outbound.UpdateCard(context.Background(), UpdateCardRequest{
			MessageID: "message-1", Card: map[string]any{"content": strings.Repeat("\n", 20<<10)},
		})
		if !errors.Is(err, ErrPayloadTooLarge) || IsRetryable(err) || api.patchCalls != 0 {
			t.Fatalf("UpdateCard() error=%v patch=%d", err, api.patchCalls)
		}
	})

}

func TestAPIErrorClassificationAndRedaction(t *testing.T) {
	t.Parallel()
	permanent := responseAPIError("create message", 230001, " invalid   request ", &larkcore.ApiResp{StatusCode: 400})
	var apiErr *APIError
	if !errors.As(permanent, &apiErr) || apiErr.Code != 230001 || apiErr.HTTPStatus != 400 || apiErr.Message != "invalid request" || !apiErr.Permanent() || IsRetryable(permanent) {
		t.Fatalf("permanent API error = %#v, %v", apiErr, permanent)
	}

	for name, err := range map[string]error{
		"rate code":   responseAPIError("reply message", 99991400, "rate limited", &larkcore.ApiResp{StatusCode: 400}),
		"http rate":   responseAPIError("reply message", 1, "rate limited", &larkcore.ApiResp{StatusCode: 429}),
		"server":      responseAPIError("patch message", 1, "unavailable", &larkcore.ApiResp{StatusCode: 503}),
		"context":     requestAPIError("patch message", context.DeadlineExceeded),
		"network-ish": requestAPIError("patch message", errors.New("connection closed before response")),
	} {
		t.Run(name, func(t *testing.T) {
			if !IsRetryable(err) {
				t.Fatalf("IsRetryable(%v) = false", err)
			}
		})
	}

	rawCause := errors.New("raw response body containing app-secret-value")
	redacted := requestAPIError("create message", rawCause)
	if !errors.Is(redacted, rawCause) || strings.Contains(redacted.Error(), "app-secret-value") || strings.Contains(redacted.Error(), "raw response body") {
		t.Fatalf("request error leaked raw cause: %v", redacted)
	}
	if IsRetryable(errors.New("local card validation failed")) {
		t.Fatal("local validation error was classified retryable")
	}
}

func TestSDKOutboundDoesNotRetryDialFailure(t *testing.T) {
	t.Parallel()
	calls := 0
	httpClient := &singleAttemptHTTPClient{client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("unreachable")}
	})}}
	client := lark.NewClient(
		"app-id",
		"app-secret",
		lark.WithOpenBaseUrl("https://open.feishu.test"),
		lark.WithEnableTokenCache(false),
		lark.WithHttpClient(httpClient),
	)
	outbound := newDirectOutbound(client, tenantTokenSourceFunc(func(context.Context) (string, error) {
		return "tenant-token", nil
	}))

	_, err := outbound.SendCard(context.Background(), SendCardRequest{
		ChatID: "chat-1", Card: map[string]any{"text": "hello"}, IdempotencyKey: "delivery-1",
	})
	if err == nil || !IsRetryable(err) {
		t.Fatalf("SendText() error = %v, want retryable transport error", err)
	}
	if calls != 1 {
		t.Fatalf("network calls = %d, want exactly one", calls)
	}
}

func TestSDKOutboundInvalidatesRejectedTokenWithoutSameAttemptRetry(t *testing.T) {
	t.Parallel()
	calls := 0
	tokens := &recordingTenantTokenSource{token: "stale-token"}
	httpClient := &singleAttemptHTTPClient{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if got := req.Header.Get("Authorization"); got != "Bearer stale-token" {
			t.Fatalf("Authorization = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"code":99991663,"msg":"tenant token expired"}`)),
			Request:    req,
		}, nil
	})}}
	client := lark.NewClient(
		"app-id",
		"app-secret",
		lark.WithOpenBaseUrl("https://open.feishu.test"),
		lark.WithEnableTokenCache(false),
		lark.WithHttpClient(httpClient),
	)
	outbound := newDirectOutbound(client, tokens)

	_, err := outbound.SendCard(context.Background(), SendCardRequest{
		ChatID: "chat-1", Card: map[string]any{"text": "hello"}, IdempotencyKey: "delivery-1",
	})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != tenantAccessTokenInvalidCode || !IsRetryable(err) {
		t.Fatalf("SendText() error = %#v, %v", apiErr, err)
	}
	if calls != 1 || tokens.invalidated != "stale-token" {
		t.Fatalf("network calls = %d, invalidated = %q", calls, tokens.invalidated)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type recordingTenantTokenSource struct {
	token       string
	invalidated string
}

func (s *recordingTenantTokenSource) Token(context.Context) (string, error) {
	return s.token, nil
}

func (s *recordingTenantTokenSource) Invalidate(token string) {
	s.invalidated = token
}

type fakeLarkOpenAPI struct {
	createImageCalls    int
	createFileCalls     int
	createCalls         int
	replyCalls          int
	updateCalls         int
	patchCalls          int
	createReactionCalls int
	deleteReactionCalls int

	createImageReq    *createImageAPIRequest
	createFileReq     *createFileAPIRequest
	createReq         *createMessageAPIRequest
	replyReq          *replyMessageAPIRequest
	patchReq          *patchMessageAPIRequest
	createReactionReq *createReactionAPIRequest
	deleteReactionReq *deleteReactionAPIRequest

	createImageErr    error
	createImageResp   *larkim.CreateImageResp
	createFileErr     error
	createFileResp    *larkim.CreateFileResp
	createErr         error
	createResp        *larkim.CreateMessageResp
	replyErr          error
	updateErr         error
	patchErr          error
	createReactionErr error
	deleteReactionErr error
}

func (f *fakeLarkOpenAPI) CreateImage(_ context.Context, req createImageAPIRequest) (*larkim.CreateImageResp, error) {
	f.createImageCalls++
	f.createImageReq = &req
	if f.createImageErr != nil {
		return nil, f.createImageErr
	}
	if f.createImageResp != nil {
		return f.createImageResp, nil
	}
	return &larkim.CreateImageResp{Data: &larkim.CreateImageRespData{ImageKey: testStringPointer("img-test")}}, nil
}

func (f *fakeLarkOpenAPI) CreateFile(_ context.Context, req createFileAPIRequest) (*larkim.CreateFileResp, error) {
	f.createFileCalls++
	f.createFileReq = &req
	if f.createFileErr != nil {
		return nil, f.createFileErr
	}
	if f.createFileResp != nil {
		return f.createFileResp, nil
	}
	return &larkim.CreateFileResp{Data: &larkim.CreateFileRespData{FileKey: testStringPointer("file-test")}}, nil
}

func (f *fakeLarkOpenAPI) CreateMessage(_ context.Context, req createMessageAPIRequest) (*larkim.CreateMessageResp, error) {
	f.createCalls++
	f.createReq = &req
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.createResp != nil {
		return f.createResp, nil
	}
	return &larkim.CreateMessageResp{Data: &larkim.CreateMessageRespData{MessageId: testStringPointer("created-message")}}, nil
}

func (f *fakeLarkOpenAPI) ReplyMessage(_ context.Context, req replyMessageAPIRequest) (*larkim.ReplyMessageResp, error) {
	f.replyCalls++
	f.replyReq = &req
	if f.replyErr != nil {
		return nil, f.replyErr
	}
	return &larkim.ReplyMessageResp{Data: &larkim.ReplyMessageRespData{MessageId: testStringPointer("reply-message")}}, nil
}

func (f *fakeLarkOpenAPI) PatchMessage(_ context.Context, req patchMessageAPIRequest) (*larkim.PatchMessageResp, error) {
	f.patchCalls++
	f.patchReq = &req
	if f.patchErr != nil {
		return nil, f.patchErr
	}
	return &larkim.PatchMessageResp{}, nil
}

func (f *fakeLarkOpenAPI) CreateMessageReaction(_ context.Context, req createReactionAPIRequest) (*larkim.CreateMessageReactionResp, error) {
	f.createReactionCalls++
	f.createReactionReq = &req
	if f.createReactionErr != nil {
		return nil, f.createReactionErr
	}
	return &larkim.CreateMessageReactionResp{Data: &larkim.CreateMessageReactionRespData{ReactionId: testStringPointer("reaction-1")}}, nil
}

func (f *fakeLarkOpenAPI) DeleteMessageReaction(_ context.Context, req deleteReactionAPIRequest) (*larkim.DeleteMessageReactionResp, error) {
	f.deleteReactionCalls++
	f.deleteReactionReq = &req
	if f.deleteReactionErr != nil {
		return nil, f.deleteReactionErr
	}
	return &larkim.DeleteMessageReactionResp{}, nil
}

func testStringPointer(value string) *string {
	return &value
}

var _ larkOpenAPI = (*fakeLarkOpenAPI)(nil)

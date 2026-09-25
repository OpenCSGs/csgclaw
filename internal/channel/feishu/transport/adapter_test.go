package transport

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type contextKey string

func TestAdapterUsesBindingLifetimeContextAndDelegatesProtocolOperations(t *testing.T) {
	lifecycle := &fakeLifecycle{identity: Identity{
		OpenID: "ou_bot", UserID: "user_bot", UnionID: "on_bot", Name: "Bot",
	}}
	oapi := &fakeOperations{}
	adapter := newAdapter(lifecycle, oapi)
	ctx := context.WithValue(context.Background(), contextKey("binding"), "binding-1")

	if _, err := adapter.SendCard(ctx, SendCardRequest{ChatID: "oc_chat", Card: map[string]any{}}); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("SendText() before Start error = %v, want ErrNotStarted", err)
	}
	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if lifecycle.startCtx != ctx || lifecycle.startCtx.Value(contextKey("binding")) != "binding-1" {
		t.Fatal("Start() did not pass the binding lifetime context through")
	}
	if got := adapter.Identity(); got != (Identity{OpenID: "ou_bot", UserID: "user_bot", UnionID: "on_bot", Name: "Bot"}) {
		t.Fatalf("Identity() = %#v", got)
	}
	if err := adapter.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Start() error = %v, want ErrAlreadyStarted", err)
	}
	cardResult, err := adapter.SendCard(ctx, SendCardRequest{
		ChatID: " oc_chat ", Card: map[string]any{"schema": "2.0"}, IdempotencyKey: " delivery-card ", ReplyTo: " om_root ", ReplyInThread: true, ThreadID: " omt_thread ",
	})
	if err != nil || cardResult.MessageID != "om_card" {
		t.Fatalf("SendCard() = %#v, %v", cardResult, err)
	}
	if oapi.sendCard.ChatID != " oc_chat " || oapi.sendCard.ThreadID != " omt_thread " || oapi.sendCard.IdempotencyKey != " delivery-card " || oapi.sendCard.Card["schema"] != "2.0" {
		t.Fatalf("SendCard request = %#v", oapi.sendCard)
	}
	if err := adapter.UpdateCard(ctx, UpdateCardRequest{MessageID: " om_card ", Card: map[string]any{"updated": true}}); err != nil {
		t.Fatalf("UpdateCard() error = %v", err)
	}
	if oapi.updateCard.MessageID != " om_card " || oapi.updateCard.Card["updated"] != true {
		t.Fatalf("UpdateCard request = %#v", oapi.updateCard)
	}

	imageUpload, err := adapter.UploadImage(ctx, UploadImageRequest{
		MediaType: " image/png ", SizeBytes: 5, Content: strings.NewReader("image"),
	})
	if err != nil || imageUpload.Key != "image-key" {
		t.Fatalf("UploadImage() = %#v, %v", imageUpload, err)
	}
	if oapi.uploadImage.MediaType != " image/png " || oapi.uploadImage.SizeBytes != 5 {
		t.Fatalf("UploadImage request = %#v", oapi.uploadImage)
	}
	imageResult, err := adapter.SendImage(ctx, SendImageRequest{
		ChatID: " oc_chat ", ImageKey: imageUpload.Key,
		IdempotencyKey: " delivery-image ", ReplyTo: " om_root ", ReplyInThread: true, ThreadID: " omt_thread ",
	})
	if err != nil || imageResult.MessageID != "om_image" {
		t.Fatalf("SendImage() = %#v, %v", imageResult, err)
	}
	if oapi.sendImage.ChatID != " oc_chat " || oapi.sendImage.ImageKey != "image-key" || oapi.sendImage.IdempotencyKey != " delivery-image " || oapi.sendImage.ThreadID != " omt_thread " {
		t.Fatalf("SendImage request = %#v", oapi.sendImage)
	}
	fileUpload, err := adapter.UploadFile(ctx, UploadFileRequest{
		Name: " report.pdf ", SizeBytes: 4, Content: strings.NewReader("file"),
	})
	if err != nil || fileUpload.Key != "file-key" {
		t.Fatalf("UploadFile() = %#v, %v", fileUpload, err)
	}
	if oapi.uploadFile.Name != " report.pdf " || oapi.uploadFile.SizeBytes != 4 {
		t.Fatalf("UploadFile request = %#v", oapi.uploadFile)
	}
	fileResult, err := adapter.SendFile(ctx, SendFileRequest{
		ChatID: " oc_chat ", FileKey: fileUpload.Key,
		IdempotencyKey: " delivery-file ", ReplyTo: " om_root ", ReplyInThread: true, ThreadID: " omt_thread ",
	})
	if err != nil || fileResult.MessageID != "om_file" {
		t.Fatalf("SendFile() = %#v, %v", fileResult, err)
	}
	if oapi.sendFile.ChatID != " oc_chat " || oapi.sendFile.FileKey != "file-key" || oapi.sendFile.IdempotencyKey != " delivery-file " || oapi.sendFile.ThreadID != " omt_thread " {
		t.Fatalf("SendFile request = %#v", oapi.sendFile)
	}

	reaction, err := adapter.AddReaction(ctx, AddReactionRequest{MessageID: " om_message ", EmojiType: " Pin "})
	if err != nil || reaction.ReactionID != "reaction-1" {
		t.Fatalf("AddReaction() = %#v, %v", reaction, err)
	}
	if oapi.addReaction.MessageID != " om_message " || oapi.addReaction.EmojiType != " Pin " {
		t.Fatalf("AddReaction request = %#v", oapi.addReaction)
	}
	if err := adapter.DeleteReaction(ctx, DeleteReactionRequest{MessageID: " om_message ", ReactionID: " reaction-1 "}); err != nil {
		t.Fatalf("DeleteReaction() error = %v", err)
	}
	if oapi.deleteReaction.MessageID != " om_message " || oapi.deleteReaction.ReactionID != " reaction-1 " {
		t.Fatalf("DeleteReaction request = %#v", oapi.deleteReaction)
	}

	download, err := adapter.DownloadResource(ctx, DownloadResourceRequest{
		MessageID: " om_message ", FileKey: " file_key ", Type: DownloadFile, DestinationPath: "/tmp/feishu-resource",
	})
	if err != nil || download.ContentType != "application/octet-stream" || download.BytesWritten != 128 {
		t.Fatalf("DownloadResource() = %#v, %v", download, err)
	}
	if oapi.download.MessageID != " om_message " || oapi.download.FileKey != " file_key " || oapi.download.Type != DownloadFile || oapi.download.DestinationPath != "/tmp/feishu-resource" {
		t.Fatalf("DownloadResource request = %#v", oapi.download)
	}

	if err := adapter.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if lifecycle.disconnectCalls != 1 {
		t.Fatalf("Disconnect calls = %d, want 1", lifecycle.disconnectCalls)
	}
	if err := adapter.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if lifecycle.disconnectCalls != 1 {
		t.Fatalf("Disconnect calls after idempotent Close = %d, want 1", lifecycle.disconnectCalls)
	}
	if _, err := adapter.SendCard(ctx, SendCardRequest{ChatID: "oc_chat", Card: map[string]any{}}); !errors.Is(err, ErrClosed) {
		t.Fatalf("SendText() after Close error = %v, want ErrClosed", err)
	}
}

func TestAdapterCloseCanBeRetriedAfterDisconnectFailure(t *testing.T) {
	disconnectErr := errors.New("temporary disconnect failure")
	lifecycle := &fakeLifecycle{disconnectErr: disconnectErr}
	adapter := newAdapter(lifecycle, &fakeOperations{})
	if err := adapter.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := adapter.Close(context.Background()); !errors.Is(err, disconnectErr) {
		t.Fatalf("Close() error = %v, want disconnect failure", err)
	}
	lifecycle.disconnectErr = nil
	if err := adapter.Close(context.Background()); err != nil {
		t.Fatalf("retry Close() error = %v", err)
	}
	if lifecycle.disconnectCalls != 2 {
		t.Fatalf("Disconnect calls = %d, want 2", lifecycle.disconnectCalls)
	}
}

func TestAdapterPreparesIdentityBeforeStart(t *testing.T) {
	lifecycle := &fakeLifecycle{identity: Identity{OpenID: "ou_bot", UserID: "user_bot"}}
	adapter := newAdapter(lifecycle, &fakeOperations{})

	identity, err := adapter.PrepareIdentity(context.Background())
	if err != nil {
		t.Fatalf("PrepareIdentity() error = %v", err)
	}
	if lifecycle.prepareCalls != 1 {
		t.Fatalf("PrepareIdentity calls = %d, want 1", lifecycle.prepareCalls)
	}
	if identity != (Identity{OpenID: "ou_bot", UserID: "user_bot"}) || adapter.Identity() != identity {
		t.Fatalf("prepared identity = %#v / %#v", identity, adapter.Identity())
	}
	if err := adapter.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if lifecycle.prepareCalls != 1 {
		t.Fatalf("Start() unexpectedly prepared identity again: %d calls", lifecycle.prepareCalls)
	}
}

type fakeLifecycle struct {
	startCtx        context.Context
	startErr        error
	disconnectErr   error
	disconnectCalls int
	identity        Identity
	prepareErr      error
	prepareCalls    int
}

func (f *fakeLifecycle) Start(ctx context.Context) error {
	f.startCtx = ctx
	return f.startErr
}

func (f *fakeLifecycle) Disconnect(context.Context) error {
	f.disconnectCalls++
	return f.disconnectErr
}

func (f *fakeLifecycle) BotIdentity() Identity {
	return f.identity
}

func (f *fakeLifecycle) PrepareIdentity(context.Context) (Identity, error) {
	f.prepareCalls++
	return f.identity, f.prepareErr
}

type fakeOperations struct {
	sendCard       SendCardRequest
	updateCard     UpdateCardRequest
	uploadImage    UploadImageRequest
	uploadFile     UploadFileRequest
	sendImage      SendImageRequest
	sendFile       SendFileRequest
	addReaction    AddReactionRequest
	deleteReaction DeleteReactionRequest
	download       DownloadResourceRequest
}

func (f *fakeOperations) SendCard(_ context.Context, req SendCardRequest) (SendResult, error) {
	f.sendCard = req
	return SendResult{MessageID: " om_card "}, nil
}

func (f *fakeOperations) UpdateCard(_ context.Context, req UpdateCardRequest) error {
	f.updateCard = req
	return nil
}

func (f *fakeOperations) UploadImage(_ context.Context, req UploadImageRequest) (UploadResult, error) {
	f.uploadImage = req
	return UploadResult{Key: " image-key "}, nil
}

func (f *fakeOperations) UploadFile(_ context.Context, req UploadFileRequest) (UploadResult, error) {
	f.uploadFile = req
	return UploadResult{Key: " file-key "}, nil
}

func (f *fakeOperations) SendImage(_ context.Context, req SendImageRequest) (SendResult, error) {
	f.sendImage = req
	return SendResult{MessageID: " om_image "}, nil
}

func (f *fakeOperations) SendFile(_ context.Context, req SendFileRequest) (SendResult, error) {
	f.sendFile = req
	return SendResult{MessageID: " om_file "}, nil
}

func (f *fakeOperations) AddReaction(_ context.Context, req AddReactionRequest) (AddReactionResult, error) {
	f.addReaction = req
	return AddReactionResult{ReactionID: " reaction-1 "}, nil
}

func (f *fakeOperations) DeleteReaction(_ context.Context, req DeleteReactionRequest) error {
	f.deleteReaction = req
	return nil
}

func (f *fakeOperations) DownloadResource(_ context.Context, req DownloadResourceRequest) (DownloadResourceResult, error) {
	f.download = req
	return DownloadResourceResult{ContentType: "application/octet-stream", BytesWritten: 128}, nil
}

type fakePublicAdapter struct{}

func (*fakePublicAdapter) Start(context.Context) error { return nil }
func (*fakePublicAdapter) Close(context.Context) error { return nil }
func (*fakePublicAdapter) Identity() Identity          { return Identity{} }

func (*fakePublicAdapter) SendCard(context.Context, SendCardRequest) (SendResult, error) {
	return SendResult{}, nil
}
func (*fakePublicAdapter) UpdateCard(context.Context, UpdateCardRequest) error { return nil }
func (*fakePublicAdapter) UploadImage(context.Context, UploadImageRequest) (UploadResult, error) {
	return UploadResult{}, nil
}
func (*fakePublicAdapter) UploadFile(context.Context, UploadFileRequest) (UploadResult, error) {
	return UploadResult{}, nil
}
func (*fakePublicAdapter) SendImage(context.Context, SendImageRequest) (SendResult, error) {
	return SendResult{}, nil
}
func (*fakePublicAdapter) SendFile(context.Context, SendFileRequest) (SendResult, error) {
	return SendResult{}, nil
}
func (*fakePublicAdapter) AddReaction(context.Context, AddReactionRequest) (AddReactionResult, error) {
	return AddReactionResult{}, nil
}
func (*fakePublicAdapter) DeleteReaction(context.Context, DeleteReactionRequest) error { return nil }
func (*fakePublicAdapter) DownloadResource(context.Context, DownloadResourceRequest) (DownloadResourceResult, error) {
	return DownloadResourceResult{}, nil
}

var _ larkLifecycle = (*fakeLifecycle)(nil)
var _ larkIdentityPreparer = (*fakeLifecycle)(nil)
var _ larkOperations = (*fakeOperations)(nil)
var _ Adapter = (*fakePublicAdapter)(nil)

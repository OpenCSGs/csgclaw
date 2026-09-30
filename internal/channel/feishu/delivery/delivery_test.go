package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"csgclaw/internal/agentengine"
	channeltypes "csgclaw/internal/channel"
	"csgclaw/internal/channel/feishu/presentation"
	feishustate "csgclaw/internal/channel/feishu/state"
	"csgclaw/internal/channel/feishu/transport"
)

type recordingAdapter struct {
	transport.Adapter
	mu           sync.Mutex
	texts        []transport.SendCardRequest
	updates      []transport.UpdateCardRequest
	imageUploads []transport.UploadImageRequest
	fileUploads  []transport.UploadFileRequest
	images       []transport.SendImageRequest
	files        []transport.SendFileRequest
	imageBody    []string
	fileBody     []string
	textErr      error
	updateErr    error
	imageErr     error
	fileErr      error
	messageID    string
}

func (a *recordingAdapter) SendCard(_ context.Context, card transport.SendCardRequest) (transport.SendResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	req := card
	a.texts = append(a.texts, req)
	if a.textErr != nil {
		return transport.SendResult{}, a.textErr
	}
	return transport.SendResult{MessageID: a.messageID}, nil
}
func (a *recordingAdapter) UpdateCard(_ context.Context, card transport.UpdateCardRequest) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	req := card
	a.updates = append(a.updates, req)
	return a.updateErr
}
func (a *recordingAdapter) UploadImage(_ context.Context, req transport.UploadImageRequest) (transport.UploadResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.imageUploads = append(a.imageUploads, req)
	if req.Content != nil {
		body, _ := io.ReadAll(req.Content)
		a.imageBody = append(a.imageBody, string(body))
	}
	return transport.UploadResult{Key: "image-key"}, nil
}
func (a *recordingAdapter) UploadFile(_ context.Context, req transport.UploadFileRequest) (transport.UploadResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fileUploads = append(a.fileUploads, req)
	if req.Content != nil {
		body, _ := io.ReadAll(req.Content)
		a.fileBody = append(a.fileBody, string(body))
	}
	return transport.UploadResult{Key: "file-key"}, nil
}
func (a *recordingAdapter) SendImage(_ context.Context, req transport.SendImageRequest) (transport.SendResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.images = append(a.images, req)
	if a.imageErr != nil {
		return transport.SendResult{}, a.imageErr
	}
	return transport.SendResult{MessageID: a.messageID}, nil
}
func (a *recordingAdapter) SendFile(_ context.Context, req transport.SendFileRequest) (transport.SendResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.files = append(a.files, req)
	if a.fileErr != nil {
		return transport.SendResult{}, a.fileErr
	}
	return transport.SendResult{MessageID: a.messageID}, nil
}

func TestDispatcherDeliversFromMemory(t *testing.T) {
	store := feishustate.NewStore()
	intent := channeltypes.DeliveryIntent{
		ID: "text", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryCard, ChatID: "chat-1", Card: presentation.Card("answer"),
	}
	if err := store.Enqueue(intent); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingAdapter{messageID: "message-1"}
	dispatcher, err := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.drain(context.Background())
	delivered, ok := store.Delivery(intent.ID)
	if !ok || delivered.Status != channeltypes.DeliveryDelivered || delivered.MessageID != "message-1" {
		t.Fatalf("delivery = %#v, found=%t", delivered, ok)
	}
}

func TestDispatcherDeliversAgentEngineMedia(t *testing.T) {
	store := feishustate.NewStore()
	image := channeltypes.DeliveryIntent{
		ID: "image", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryFile, ChatID: "chat-1", ReplyTo: "root-1", ThreadID: "thread-1",
		FileID: "file-image",
	}
	file := channeltypes.DeliveryIntent{
		ID: "file", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryFile, ChatID: "chat-1",
		FileID: "file-report",
	}
	if err := store.Enqueue(image); err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(file); err != nil {
		t.Fatal(err)
	}
	resolver := fileResolverFunc(func(_ context.Context, fileID string) (agentengine.FileContent, error) {
		switch fileID {
		case "file-image":
			return agentengine.FileContent{
				Metadata: agentengine.OutputFileMetadata{ID: fileID, Name: "result.png", MediaType: "image/png", SizeBytes: 3},
				Content:  io.NopCloser(strings.NewReader("png")),
			}, nil
		case "file-report":
			return agentengine.FileContent{
				Metadata: agentengine.OutputFileMetadata{ID: fileID, Name: "report.pdf", MediaType: "application/pdf", SizeBytes: 3},
				Content:  io.NopCloser(strings.NewReader("pdf")),
			}, nil
		default:
			t.Fatalf("unexpected fileID = %q", fileID)
			return agentengine.FileContent{}, nil
		}
	})
	adapter := &recordingAdapter{messageID: "message-media"}
	dispatcher, err := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter, Files: resolver})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.drain(context.Background())

	deliveredImage, _ := store.Delivery(image.ID)
	deliveredFile, _ := store.Delivery(file.ID)
	if deliveredImage.Status != channeltypes.DeliveryDelivered || deliveredImage.MessageID != "message-media" ||
		deliveredFile.Status != channeltypes.DeliveryDelivered || deliveredFile.MessageID != "message-media" {
		t.Fatalf("image=%#v file=%#v", deliveredImage, deliveredFile)
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.imageUploads) != 1 || len(adapter.images) != 1 || len(adapter.imageBody) != 1 ||
		adapter.imageUploads[0].MediaType != "image/png" || adapter.images[0].ImageKey != "image-key" || adapter.images[0].ReplyTo != "root-1" ||
		!adapter.images[0].ReplyInThread || adapter.images[0].ThreadID != "thread-1" || adapter.imageBody[0] != "png" {
		t.Fatalf("image uploads=%#v sends=%#v bodies=%#v", adapter.imageUploads, adapter.images, adapter.imageBody)
	}
	if len(adapter.fileUploads) != 1 || len(adapter.files) != 1 || len(adapter.fileBody) != 1 ||
		adapter.fileUploads[0].Name != "report.pdf" || adapter.files[0].FileKey != "file-key" || adapter.fileBody[0] != "pdf" {
		t.Fatalf("file uploads=%#v sends=%#v bodies=%#v", adapter.fileUploads, adapter.files, adapter.fileBody)
	}
}

func TestDispatcherSendsOversizedImageAsFile(t *testing.T) {
	store := feishustate.NewStore()
	intent := channeltypes.DeliveryIntent{
		ID: "large-image", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryFile, ChatID: "chat-1", FileID: "file-large-image",
	}
	if err := store.Enqueue(intent); err != nil {
		t.Fatal(err)
	}
	resolver := fileResolverFunc(func(_ context.Context, fileID string) (agentengine.FileContent, error) {
		return agentengine.FileContent{
			Metadata: agentengine.OutputFileMetadata{
				ID: fileID, Name: "large.png", MediaType: "image/png", SizeBytes: transport.ImageUploadLimitBytes + 1,
			},
			Content: io.NopCloser(strings.NewReader("png")),
		}, nil
	})
	adapter := &recordingAdapter{messageID: "message-large-image"}
	dispatcher, err := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter, Files: resolver})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.drain(context.Background())

	delivered, _ := store.Delivery(intent.ID)
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if delivered.Status != channeltypes.DeliveryDelivered || len(adapter.imageUploads) != 0 ||
		len(adapter.fileUploads) != 1 || adapter.fileUploads[0].Name != "large.png" || len(adapter.files) != 1 {
		t.Fatalf("delivery=%#v image uploads=%d file uploads=%#v sends=%#v", delivered, len(adapter.imageUploads), adapter.fileUploads, adapter.files)
	}
}

func TestDispatcherReusesUploadWhenMessageSendRetries(t *testing.T) {
	store := feishustate.NewStore()
	intent := channeltypes.DeliveryIntent{
		ID: "retry-image", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryFile, ChatID: "chat-1", FileID: "file-image",
	}
	if err := store.Enqueue(intent); err != nil {
		t.Fatal(err)
	}
	resolveCalls := 0
	resolver := fileResolverFunc(func(_ context.Context, fileID string) (agentengine.FileContent, error) {
		resolveCalls++
		return agentengine.FileContent{
			Metadata: agentengine.OutputFileMetadata{ID: fileID, Name: "result.png", MediaType: "image/png", SizeBytes: 3},
			Content:  io.NopCloser(strings.NewReader("png")),
		}, nil
	})
	adapter := &recordingAdapter{
		messageID: "message-image",
		imageErr:  &transport.APIError{Operation: "send image", HTTPStatus: 503},
	}
	dispatcher, err := NewDispatcher(DispatcherOptions{
		State: store, Adapter: adapter, Files: resolver, RetryInterval: time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.drain(context.Background())
	adapter.mu.Lock()
	adapter.imageErr = nil
	adapter.mu.Unlock()
	time.Sleep(time.Millisecond)
	dispatcher.drain(context.Background())

	delivered, _ := store.Delivery(intent.ID)
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if delivered.Status != channeltypes.DeliveryDelivered || resolveCalls != 1 ||
		len(adapter.imageUploads) != 1 || len(adapter.images) != 2 {
		t.Fatalf("delivery=%#v resolve=%d uploads=%d sends=%d", delivered, resolveCalls, len(adapter.imageUploads), len(adapter.images))
	}
}

func TestDispatcherRejectsIncompleteEngineMetadataWithoutFallback(t *testing.T) {
	store := feishustate.NewStore()
	intent := channeltypes.DeliveryIntent{
		ID: "invalid-metadata", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryFile, ChatID: "chat-1", FileID: "file-invalid",
	}
	if err := store.Enqueue(intent); err != nil {
		t.Fatal(err)
	}
	resolver := fileResolverFunc(func(context.Context, string) (agentengine.FileContent, error) {
		return agentengine.FileContent{
			Metadata: agentengine.OutputFileMetadata{Name: "result.png", MediaType: "image/png", SizeBytes: 3},
			Content:  io.NopCloser(strings.NewReader("png")),
		}, nil
	})
	adapter := &recordingAdapter{}
	dispatcher, err := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter, Files: resolver})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.drain(context.Background())

	failed, _ := store.Delivery(intent.ID)
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if failed.Status != channeltypes.DeliveryFailed || len(adapter.imageUploads) != 0 || len(adapter.fileUploads) != 0 {
		t.Fatalf("delivery=%#v image uploads=%d file uploads=%d", failed, len(adapter.imageUploads), len(adapter.fileUploads))
	}
}

func TestDispatcherResolvesInMemoryUpdateDependency(t *testing.T) {
	store := feishustate.NewStore()
	create := channeltypes.DeliveryIntent{
		ID: "create", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryCard, ChatID: "chat-1", Card: presentation.Card("working"),
	}
	update := channeltypes.DeliveryIntent{
		ID: "update", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryCardUpdate, RelatedID: create.ID, Card: presentation.Card("done"),
	}
	if err := store.Enqueue(create); err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(update); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingAdapter{messageID: "message-1"}
	dispatcher, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter})
	dispatcher.drain(context.Background())
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if len(adapter.texts) != 1 || len(adapter.updates) != 1 || adapter.updates[0].MessageID != "message-1" {
		t.Fatalf("texts=%#v updates=%#v", adapter.texts, adapter.updates)
	}
}

func TestDispatcherUsesBoundedInProcessRetry(t *testing.T) {
	store := feishustate.NewStore()
	intent := channeltypes.DeliveryIntent{
		ID: "retry", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryCard, ChatID: "chat-1", Card: presentation.Card("answer"),
	}
	if err := store.Enqueue(intent); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingAdapter{textErr: &transport.APIError{Operation: "send", HTTPStatus: 503}}
	dispatcher, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter, RetryInterval: time.Nanosecond})
	for attempt := 0; attempt < maxDeliveryAttempts+1; attempt++ {
		dispatcher.drain(context.Background())
		time.Sleep(time.Millisecond)
	}
	failed, _ := store.Delivery(intent.ID)
	if failed.Status != channeltypes.DeliveryFailed || failed.Attempts != maxDeliveryAttempts {
		t.Fatalf("delivery = %#v", failed)
	}
}

func TestDispatcherDoesNotRetryPermanentFailure(t *testing.T) {
	store := feishustate.NewStore()
	intent := channeltypes.DeliveryIntent{
		ID: "permanent", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryCard, ChatID: "chat-1", Card: presentation.Card("answer"),
	}
	if err := store.Enqueue(intent); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingAdapter{textErr: &transport.APIError{Operation: "send", HTTPStatus: 400}}
	dispatcher, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter, RetryInterval: time.Nanosecond})
	dispatcher.drain(context.Background())
	dispatcher.drain(context.Background())
	failed, _ := store.Delivery(intent.ID)
	if failed.Status != channeltypes.DeliveryFailed || failed.Attempts != 1 {
		t.Fatalf("delivery = %#v", failed)
	}
	if got := len(adapter.texts); got != 1 {
		t.Fatalf("send attempts = %d, want 1", got)
	}
}

func TestRetryPolicyDoesNotRepeatAmbiguousUnsafeOperation(t *testing.T) {
	retryable := &transport.APIError{Operation: "send", HTTPStatus: 503}
	for _, kind := range []channeltypes.DeliveryKind{
		channeltypes.DeliveryReactionAdd,
		channeltypes.DeliveryCommentReply,
	} {
		if retryableDelivery(channeltypes.DeliveryIntent{Kind: kind}, retryable) {
			t.Fatalf("kind %q was unexpectedly retryable", kind)
		}
	}
	if !retryableDelivery(channeltypes.DeliveryIntent{Kind: channeltypes.DeliveryCard}, retryable) {
		t.Fatal("stable-UUID message send was not retryable")
	}
}

func TestDispatcherPreventsOldStreamUpdateAfterTerminal(t *testing.T) {
	store := feishustate.NewStore()
	create := channeltypes.DeliveryIntent{
		ID: "turn-1:reply:create", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryCard, ChatID: "chat-1", Card: presentation.Card("working"),
	}
	if err := store.Enqueue(create); err != nil {
		t.Fatal(err)
	}
	create.MessageID = "message-1"
	if err := store.MarkDelivered(create); err != nil {
		t.Fatal(err)
	}
	old := channeltypes.DeliveryIntent{
		ID: "turn-1:reply:update:1", BindingID: "binding-1", TurnID: "turn-1", Sequence: 1,
		Kind: channeltypes.DeliveryCardUpdate, RelatedID: create.ID, Card: presentation.Card("old"),
	}
	terminal := channeltypes.DeliveryIntent{
		ID: "turn-1:reply:final", BindingID: "binding-1", TurnID: "turn-1", Sequence: 2,
		Kind: channeltypes.DeliveryCardUpdate, RelatedID: create.ID, Card: presentation.Card("done"),
	}
	if err := store.Enqueue(old); err != nil {
		t.Fatal(err)
	}
	if err := store.Enqueue(terminal); err != nil {
		t.Fatal(err)
	}
	adapter := &recordingAdapter{}
	dispatcher, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter})
	dispatcher.drain(context.Background())
	stale, _ := store.Delivery(old.ID)
	if stale.Status != channeltypes.DeliveryFailed {
		t.Fatalf("old update = %#v", stale)
	}
	if len(adapter.updates) != 1 || !strings.Contains(testCardText(adapter.updates[0].Card), "done") {
		t.Fatalf("updates = %#v, want terminal only", adapter.updates)
	}
}

func TestDependencyErrorsRemainRecognizable(t *testing.T) {
	store := feishustate.NewStore()
	dispatcher, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: &recordingAdapter{}})
	update := channeltypes.DeliveryIntent{
		ID: "update", BindingID: "binding-1", TurnID: "turn-1",
		Kind: channeltypes.DeliveryCardUpdate, RelatedID: "missing", Card: presentation.Card("done"),
	}
	if _, err := dispatcher.deliverCardUpdate(context.Background(), update); !errors.Is(err, ErrDependencyTerminal) {
		t.Fatalf("dependency error = %v", err)
	}
}

func testCardText(card map[string]any) string { raw, _ := json.Marshal(card); return string(raw) }

func TestReplyPageWaitsForPreviousCard(t *testing.T) {
	store := feishustate.NewStore()
	previous := channeltypes.DeliveryIntent{ID: "page-1", BindingID: "binding", TurnID: "turn", Kind: channeltypes.DeliveryCard}
	if err := store.Enqueue(previous); err != nil {
		t.Fatal(err)
	}
	next := channeltypes.DeliveryIntent{ID: "page-2", BindingID: "binding", TurnID: "turn", Kind: channeltypes.DeliveryCard, RelatedID: previous.ID}
	adapter := &recordingAdapter{messageID: "second-message"}
	dispatcher, _ := NewDispatcher(DispatcherOptions{State: store, Adapter: adapter})
	if _, err := dispatcher.deliver(context.Background(), next); !errors.Is(err, ErrDependencyPending) {
		t.Fatalf("pending previous page: %v", err)
	}
	if len(adapter.texts) != 0 {
		t.Fatal("sent next page before previous page")
	}
	previous.MessageID = "first-message"
	if err := store.MarkDelivered(previous); err != nil {
		t.Fatal(err)
	}
	if _, err := dispatcher.deliver(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if len(adapter.texts) != 1 {
		t.Fatalf("sent %d pages", len(adapter.texts))
	}
}

package transport

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var errNilContext = errors.New("feishu transport context is required")

type larkLifecycle interface {
	Start(context.Context) error
	Disconnect(context.Context) error
	BotIdentity() Identity
}

type larkIdentityPreparer interface {
	PrepareIdentity(context.Context) (Identity, error)
}

type larkOperations interface {
	SendCard(context.Context, SendCardRequest) (SendResult, error)
	UpdateCard(context.Context, UpdateCardRequest) error
	UploadImage(context.Context, UploadImageRequest) (UploadResult, error)
	UploadFile(context.Context, UploadFileRequest) (UploadResult, error)
	SendImage(context.Context, SendImageRequest) (SendResult, error)
	SendFile(context.Context, SendFileRequest) (SendResult, error)
	AddReaction(context.Context, AddReactionRequest) (AddReactionResult, error)
	DeleteReaction(context.Context, DeleteReactionRequest) error
}

type resourceDownloader interface {
	DownloadResource(context.Context, DownloadResourceRequest) (DownloadResourceResult, error)
}

type adapter struct {
	mu         sync.RWMutex
	lifecycle  larkLifecycle
	oapi       larkOperations
	downloader resourceDownloader
	comments   CommentAdapter
	messages   MessageAdapter
	identity   Identity
	started    bool
	closed     bool
}

func newAdapter(lifecycle larkLifecycle, oapi larkOperations) *adapter {
	downloader, _ := oapi.(resourceDownloader)
	return &adapter{lifecycle: lifecycle, oapi: oapi, downloader: downloader}
}

func newAdapterWithDownloader(lifecycle larkLifecycle, oapi larkOperations, downloader resourceDownloader) *adapter {
	return &adapter{lifecycle: lifecycle, oapi: oapi, downloader: downloader}
}

func newAdapterWithDependencies(lifecycle larkLifecycle, oapi larkOperations, downloader resourceDownloader, comments CommentAdapter, messages MessageAdapter) *adapter {
	return &adapter{lifecycle: lifecycle, oapi: oapi, downloader: downloader, comments: comments, messages: messages}
}

func (a *adapter) Start(ctx context.Context) error {
	if ctx == nil {
		return errNilContext
	}
	if a == nil || a.lifecycle == nil || a.oapi == nil {
		return ErrInvalidConfig
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return ErrClosed
	}
	if a.started {
		return ErrAlreadyStarted
	}
	// The caller must pass the binding worker's lifetime context. The SDK derives
	// the WebSocket lifetime from this exact context.
	if err := a.lifecycle.Start(ctx); err != nil {
		return fmt.Errorf("start feishu transport: %w", err)
	}
	a.identity = a.lifecycle.BotIdentity()
	a.started = true
	return nil
}

func (a *adapter) PrepareIdentity(ctx context.Context) (Identity, error) {
	if ctx == nil {
		return Identity{}, errNilContext
	}
	if a == nil || a.lifecycle == nil {
		return Identity{}, ErrInvalidConfig
	}

	a.mu.RLock()
	closed := a.closed
	a.mu.RUnlock()
	if closed {
		return Identity{}, ErrClosed
	}
	preparer, ok := a.lifecycle.(larkIdentityPreparer)
	if !ok {
		return a.Identity(), nil
	}
	identity, err := preparer.PrepareIdentity(ctx)
	if err != nil {
		return Identity{}, fmt.Errorf("prepare feishu bot identity: %w", err)
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return Identity{}, ErrClosed
	}
	a.identity = identity
	a.mu.Unlock()
	return identity, nil
}

func (a *adapter) Close(ctx context.Context) error {
	if ctx == nil {
		return errNilContext
	}
	if a == nil || a.lifecycle == nil {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	if err := a.lifecycle.Disconnect(ctx); err != nil {
		return fmt.Errorf("close feishu transport: %w", err)
	}
	a.started = false
	a.closed = true
	return nil
}

func (a *adapter) Identity() Identity {
	if a == nil {
		return Identity{}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.identity
}

func (a *adapter) ResolveCommentTarget(ctx context.Context, fileToken, fileType string) (CommentTarget, bool, error) {
	if err := a.ready(ctx); err != nil {
		return CommentTarget{}, false, err
	}
	if a.comments == nil {
		return CommentTarget{}, false, ErrUnsupportedEvent
	}
	return a.comments.ResolveCommentTarget(ctx, fileToken, fileType)
}

func (a *adapter) FetchComment(ctx context.Context, target CommentTarget, commentID string) (CommentThread, error) {
	if err := a.ready(ctx); err != nil {
		return CommentThread{}, err
	}
	if a.comments == nil {
		return CommentThread{}, ErrUnsupportedEvent
	}
	return a.comments.FetchComment(ctx, target, commentID)
}

func (a *adapter) ReplyToComment(ctx context.Context, req ReplyCommentRequest) error {
	if err := a.ready(ctx); err != nil {
		return err
	}
	if a.comments == nil {
		return ErrUnsupportedEvent
	}
	return a.comments.ReplyToComment(ctx, req)
}

func (a *adapter) FetchMessage(ctx context.Context, messageID string) (Message, bool, error) {
	if err := a.ready(ctx); err != nil {
		return Message{}, false, err
	}
	if a.messages == nil {
		return Message{}, false, ErrUnsupportedEvent
	}
	return a.messages.FetchMessage(ctx, messageID)
}

func (a *adapter) SendCard(ctx context.Context, req SendCardRequest) (SendResult, error) {
	if err := a.ready(ctx); err != nil {
		return SendResult{}, err
	}
	result, err := a.oapi.SendCard(ctx, req)
	if err != nil {
		return SendResult{}, fmt.Errorf("send feishu card: %w", err)
	}
	result.MessageID = strings.TrimSpace(result.MessageID)
	return result, nil
}

func (a *adapter) UpdateCard(ctx context.Context, req UpdateCardRequest) error {
	if err := a.ready(ctx); err != nil {
		return err
	}
	if err := a.oapi.UpdateCard(ctx, req); err != nil {
		return fmt.Errorf("update feishu card: %w", err)
	}
	return nil
}

func (a *adapter) UploadImage(ctx context.Context, req UploadImageRequest) (UploadResult, error) {
	if err := a.ready(ctx); err != nil {
		return UploadResult{}, err
	}
	result, err := a.oapi.UploadImage(ctx, req)
	if err != nil {
		return UploadResult{}, fmt.Errorf("upload feishu image: %w", err)
	}
	result.Key = strings.TrimSpace(result.Key)
	return result, nil
}

func (a *adapter) UploadFile(ctx context.Context, req UploadFileRequest) (UploadResult, error) {
	if err := a.ready(ctx); err != nil {
		return UploadResult{}, err
	}
	result, err := a.oapi.UploadFile(ctx, req)
	if err != nil {
		return UploadResult{}, fmt.Errorf("upload feishu file: %w", err)
	}
	result.Key = strings.TrimSpace(result.Key)
	return result, nil
}

func (a *adapter) SendImage(ctx context.Context, req SendImageRequest) (SendResult, error) {
	if err := a.ready(ctx); err != nil {
		return SendResult{}, err
	}
	result, err := a.oapi.SendImage(ctx, req)
	if err != nil {
		return SendResult{}, fmt.Errorf("send feishu image: %w", err)
	}
	result.MessageID = strings.TrimSpace(result.MessageID)
	return result, nil
}

func (a *adapter) SendFile(ctx context.Context, req SendFileRequest) (SendResult, error) {
	if err := a.ready(ctx); err != nil {
		return SendResult{}, err
	}
	result, err := a.oapi.SendFile(ctx, req)
	if err != nil {
		return SendResult{}, fmt.Errorf("send feishu file: %w", err)
	}
	result.MessageID = strings.TrimSpace(result.MessageID)
	return result, nil
}

func (a *adapter) AddReaction(ctx context.Context, req AddReactionRequest) (AddReactionResult, error) {
	if err := a.ready(ctx); err != nil {
		return AddReactionResult{}, err
	}
	result, err := a.oapi.AddReaction(ctx, req)
	if err != nil {
		return AddReactionResult{}, fmt.Errorf("add feishu reaction: %w", err)
	}
	result.ReactionID = strings.TrimSpace(result.ReactionID)
	return result, nil
}

func (a *adapter) DeleteReaction(ctx context.Context, req DeleteReactionRequest) error {
	if err := a.ready(ctx); err != nil {
		return err
	}
	if err := a.oapi.DeleteReaction(ctx, req); err != nil {
		return fmt.Errorf("delete feishu reaction: %w", err)
	}
	return nil
}

func (a *adapter) DownloadResource(ctx context.Context, req DownloadResourceRequest) (DownloadResourceResult, error) {
	if err := a.ready(ctx); err != nil {
		return DownloadResourceResult{}, err
	}
	if a.downloader == nil {
		return DownloadResourceResult{}, errors.New("feishu resource downloader is unavailable")
	}
	result, err := a.downloader.DownloadResource(ctx, req)
	if err != nil {
		return DownloadResourceResult{}, fmt.Errorf("download feishu resource: %w", err)
	}
	return result, nil
}

func (a *adapter) ready(ctx context.Context) error {
	if ctx == nil {
		return errNilContext
	}
	if a == nil || a.oapi == nil {
		return ErrInvalidConfig
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return ErrClosed
	}
	if !a.started {
		return ErrNotStarted
	}
	return nil
}

var _ Adapter = (*adapter)(nil)
var _ IdentityPreparer = (*adapter)(nil)
var _ CommentAdapter = (*adapter)(nil)
var _ MessageAdapter = (*adapter)(nil)

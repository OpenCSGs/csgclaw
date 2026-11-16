package browser

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/proto"
)

// These are top-level browser sessions, so history navigation uses the CDP
// history entry and waits for its document or same-document commit. Returning
// immediately after history.back() can expose the previous document to tools.
func navigateHistory(page *rod.Page, delta int) error {
	page, cancel := page.WithCancel()
	defer cancel()
	history, err := readDocument(page.GetContext(), page.GetNavigationHistory)
	if err != nil {
		return fmt.Errorf("read browser history: %w", err)
	}
	index := history.CurrentIndex + delta
	if index < 0 || index >= len(history.Entries) {
		return nil
	}
	wait := page.EachEvent(
		func(event *proto.PageFrameNavigated) bool { return event.Frame.ID == page.FrameID },
		func(event *proto.PageNavigatedWithinDocument) bool { return event.FrameID == page.FrameID },
	)
	if err = (proto.PageNavigateToHistoryEntry{EntryID: history.Entries[index].ID}).Call(page); err != nil {
		return fmt.Errorf("navigate browser history: %w", err)
	}
	wait()
	if err = page.GetContext().Err(); err != nil {
		return err
	}
	if err = waitDocument(page); err != nil {
		return fmt.Errorf("wait for history document: %w", err)
	}
	return nil
}

// Only repeat document reads during Chromium's document-swap window. Input and
// navigation actions are never retried, because they may already have occurred.
func readDocument[T any](ctx context.Context, read func() (T, error)) (T, error) {
	for {
		value, err := read()
		if err == nil || (!errors.Is(err, cdp.ErrNotAttachedToActivePage) && !errors.Is(err, cdp.ErrCtxNotFound) && !errors.Is(err, cdp.ErrCtxDestroyed)) {
			return value, err
		}
		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func waitDocument(page *rod.Page) error {
	for {
		state, err := readDocument(page.GetContext(), func() (*proto.RuntimeEvaluateResult, error) {
			return (proto.RuntimeEvaluate{Expression: "document.readyState", ReturnByValue: true}).Call(page)
		})
		if err != nil {
			return err
		}
		if state.ExceptionDetails == nil && (state.Result.Value.Str() == "interactive" || state.Result.Value.Str() == "complete") {
			return nil
		}
		select {
		case <-page.GetContext().Done():
			return page.GetContext().Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

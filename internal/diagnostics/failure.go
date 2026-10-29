package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"syscall"
)

// ReportFailure records a recoverable operation error without changing the
// turn result. The product owner still decides whether execution can continue.
func ReportFailure(ctx context.Context, stage string, err error, fields map[string]any) {
	if err == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	detail := map[string]any{"stage": stage, "error": err.Error(), "error_type": fmt.Sprintf("%T", err), "error_kind": "operation_failed"}
	for key, value := range fields {
		detail[key] = value
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		detail["error_kind"] = "not_found"
	case errors.Is(err, os.ErrPermission):
		detail["error_kind"] = "permission_denied"
	case errors.Is(err, context.Canceled):
		detail["error_kind"] = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		detail["error_kind"] = "deadline_exceeded"
	case errors.Is(err, io.ErrUnexpectedEOF):
		detail["error_kind"] = "unexpected_eof"
	}
	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		detail["error_kind"] = "invalid_json"
	}
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		detail["path"] = pathError.Path
		detail["operation"] = pathError.Op
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		detail["os_error_code"] = int(errno)
	}
	Capture(ctx, "execution.failure", detail)
	if ResolveRecord(ctx) == nil {
		if lookup, ok := ctx.Value(nativeLookupKey{}).(nativeLookup); ok && lookup.store != nil && lookup.agent != "" && lookup.session != "" && lookup.turn != "" {
			return
		}
	}
	slog.WarnContext(ctx, "turn operation failed", "stage", stage, "error", ProtectText(err.Error()), "details", Protect(detail))
}

func ResolveRecord(ctx context.Context) *Record {
	if ctx == nil {
		return nil
	}
	if record := From(ctx); record != nil {
		return record
	}
	if lookup, ok := ctx.Value(nativeLookupKey{}).(nativeLookup); ok && lookup.store != nil {
		return lookup.store.native(lookup.agent, lookup.session, lookup.turn, false)
	}
	return nil
}

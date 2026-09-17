package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// WithPlatformFile opens one regular workspace file for an authorized active
// turn. The reader is valid only during consume and is bounded to its checked
// size. No absolute host path or file contents are returned to the model.
func (r *Runtime) WithPlatformFile(ctx context.Context, agentID, threadID, turnID, path string, consume func(context.Context, string, int64, io.Reader) (json.RawMessage, error)) (json.RawMessage, error) {
	if consume == nil {
		return nil, fmt.Errorf("file consumer is required")
	}
	_, live, err := r.activeAgentSession(agentID)
	if err != nil {
		return nil, err
	}
	if live.spec.ExecutionMode == ExecutionModeReadOnly {
		return nil, fmt.Errorf("file upload is unavailable in read-only mode")
	}
	turnCtx, _, err := r.AgentTurnContext(agentID, threadID, turnID)
	if err != nil {
		return nil, err
	}
	operationCtx, cancel := context.WithCancel(turnCtx)
	defer cancel()
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
	}
	var result json.RawMessage
	err = withAppServerUploadFile(live.spec, path, func(file *os.File, size int64) error {
		if err := operationCtx.Err(); err != nil {
			return err
		}
		body := io.LimitReader(platformFileReader{ctx: operationCtx, source: file}, size)
		var err error
		result, err = consume(operationCtx, filepath.Base(strings.TrimSpace(path)), size, body)
		return err
	})
	return result, err
}

type platformFileReader struct {
	ctx    context.Context
	source io.Reader
}

func (r platformFileReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}

// Native and App-backed uploads share the same filesystem boundary, including
// nonblocking opens so a FIFO cannot stall a request before regular-file checks.
func withAppServerUploadFile(spec SessionSpec, path string, consume func(*os.File, int64) error) error {
	path = strings.TrimSpace(path)
	cleaned := filepath.Clean(path)
	if path == "" || cleaned == "." || filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path must stay within the Runtime workspace")
	}
	root, err := os.OpenRoot(spec.WorkspaceDir)
	if err != nil {
		return fmt.Errorf("open Runtime workspace: %w", err)
	}
	defer root.Close()
	file, err := openAppServerUploadFile(root, cleaned)
	if err != nil {
		return fmt.Errorf("open upload file: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("upload path must reference a regular file")
	}
	if info.Size() > appServerMaxUploadBytes {
		return fmt.Errorf("upload file exceeds %d bytes", appServerMaxUploadBytes)
	}
	return consume(file, info.Size())
}

package dsh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	agentruntime "csgclaw/internal/runtime"
)

func (r *Runtime) StreamLogs(ctx context.Context, h agentruntime.Handle, opts agentruntime.LogOptions) error {
	if opts.Writer == nil {
		return fmt.Errorf("DSH log writer is required")
	}
	root, err := r.rootFor(h)
	if err != nil {
		return err
	}
	path := filepath.Join(root, stderrFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	start := tailStart(data, opts.Tail)
	if _, err := opts.Writer.Write(data[start:]); err != nil {
		return err
	}
	if !opts.Follow {
		return nil
	}
	offset := int64(len(data))
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			info, err := file.Stat()
			if err != nil {
				file.Close()
				return err
			}
			if info.Size() < offset {
				offset = 0
			}
			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				file.Close()
				return err
			}
			written, copyErr := io.Copy(opts.Writer, file)
			closeErr := file.Close()
			offset += written
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
}

func tailStart(data []byte, lines int) int {
	if lines <= 0 || len(data) == 0 {
		return 0
	}
	trimmed := bytes.TrimSuffix(data, []byte{'\n'})
	start := len(trimmed)
	for count := 0; count < lines && start > 0; count++ {
		index := bytes.LastIndexByte(trimmed[:start], '\n')
		if index < 0 {
			return 0
		}
		start = index
	}
	if start < len(trimmed) && data[start] == '\n' {
		start++
	}
	return start
}

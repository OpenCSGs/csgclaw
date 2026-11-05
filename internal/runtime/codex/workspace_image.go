package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	workerworkspace "csgclaw/internal/runtime/workspace"
)

func (a *ConversationAdapter) workspacePromptBlocks(ctx context.Context, workspace string, ref *contract.WorkspaceFile) ([]PromptContentBlock, string, error) {
	file, prompt, err := workerworkspace.OpenInput(workspace, ref)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	blocks := []PromptContentBlock{TextBlock(prompt)}
	if !strings.HasPrefix(strings.ToLower(ref.MIMEType), "image/") {
		return blocks, "", nil
	}
	info, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if info.Size() > apitypes.MaxAttachmentFileBytes {
		return blocks, "", nil
	}
	runtimeDir, err := a.runtime.RuntimeDir(a.runtimeID)
	if err != nil {
		return nil, "", err
	}
	previewDir, err := os.MkdirTemp(runtimeDir, ".input-image-")
	if err != nil {
		return nil, "", err
	}
	sourcePath := filepath.Join(workspace, filepath.FromSlash(ref.Path))
	preview := filepath.Join(previewDir, "image"+filepath.Ext(sourcePath))
	if err := prepareWorkspaceImage(ctx, file, info, sourcePath, preview); err != nil {
		_ = os.RemoveAll(previewDir)
		return nil, "", err
	}
	return append(blocks, LocalImageBlock(preview)), previewDir, nil
}

// The app-server reopens localImage by name. Pin the opened workspace file
// in the private runtime directory so renaming a workspace entry cannot
// redirect the preview. Separate filesystems require a streamed copy.
func prepareWorkspaceImage(ctx context.Context, source *os.File, info os.FileInfo, sourcePath, preview string) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("workspace image must be a regular file")
	}
	if err := os.Link(sourcePath, preview); err == nil {
		linked, err := os.Lstat(preview)
		if err == nil && linked.Mode().IsRegular() && os.SameFile(info, linked) {
			return nil
		}
		if err := os.Remove(preview); err != nil {
			return err
		}
	}
	target, err := os.OpenFile(preview, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = io.CopyN(target, platformFileReader{ctx: ctx, source: source}, info.Size())
	return errors.Join(err, target.Close())
}

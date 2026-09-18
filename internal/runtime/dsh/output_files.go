package dsh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/agentengine/contract"
)

func authorizePresentedFiles(ctx context.Context, runtimeID, workspace string, requests []presentedFile) ([]*contract.OutputFile, *contract.TurnError) {
	if len(requests) == 0 {
		return nil, nil
	}
	root, err := os.OpenRoot(strings.TrimSpace(workspace))
	if err != nil {
		return nil, dshOutputFileUnavailable(runtimeID, "open_workspace", err)
	}
	defer root.Close()

	files := make([]*contract.OutputFile, 0, len(requests))
	for _, request := range requests {
		relativePath, pathErr := presentedFileRelativePath(workspace, request.Path)
		if pathErr != nil {
			cleanupPresentedFiles(files)
			return nil, &contract.TurnError{Code: contract.ErrorFileUnavailable, Message: pathErr.Error()}
		}
		source, info, openErr := openPresentedFile(root, relativePath)
		if openErr != nil {
			cleanupPresentedFiles(files)
			return nil, dshOutputFileUnavailable(runtimeID, "open_source", openErr)
		}
		file, snapshotErr := contract.NewOutputFile(ctx, contract.OutputFileMetadata{
			Name: filepath.Base(relativePath), MediaType: "application/octet-stream", SizeBytes: info.Size(),
		}, source)
		closeErr := source.Close()
		if snapshotErr != nil || closeErr != nil {
			if file != nil {
				file.Cleanup()
			}
			cleanupPresentedFiles(files)
			return nil, dshOutputFileUnavailable(runtimeID, "snapshot_source", errors.Join(snapshotErr, closeErr))
		}
		mediaType, detectErr := detectPresentedFileMediaType(ctx, file)
		if detectErr != nil {
			file.Cleanup()
			cleanupPresentedFiles(files)
			return nil, dshOutputFileUnavailable(runtimeID, "detect_media_type", detectErr)
		}
		file.MediaType = mediaType
		files = append(files, file)
	}
	return files, nil
}

func presentedFileRelativePath(workspace, path string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("DSH presented file path is empty")
	}
	if filepath.IsAbs(path) {
		relative, err := filepath.Rel(workspace, filepath.Clean(path))
		if err != nil {
			return "", fmt.Errorf("DSH presented file must stay within the Runtime workspace")
		}
		path = relative
	}
	cleaned := filepath.Clean(path)
	if cleaned == "." || filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" ||
		cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("DSH presented file must stay within the Runtime workspace")
	}
	return cleaned, nil
}

func openPresentedFile(root *os.Root, relativePath string) (*os.File, os.FileInfo, error) {
	info, err := root.Lstat(relativePath)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect DSH presented file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("DSH presented output must be a regular non-symlink file")
	}
	file, err := root.Open(relativePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open DSH presented file: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, nil, fmt.Errorf("DSH presented file changed before it could be opened")
	}
	return file, openedInfo, nil
}

func detectPresentedFileMediaType(ctx context.Context, file *contract.OutputFile) (string, error) {
	download, err := file.Open(ctx)
	if err != nil {
		return "", err
	}
	prefix := make([]byte, 512)
	prefixBytes, readErr := io.ReadFull(download, prefix)
	closeErr := download.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) || closeErr != nil {
		return "", errors.Join(readErr, closeErr)
	}
	if byExtension := mime.TypeByExtension(filepath.Ext(file.Name)); byExtension != "" {
		parsed, _, parseErr := mime.ParseMediaType(byExtension)
		if parseErr == nil && strings.TrimSpace(parsed) != "" {
			return strings.ToLower(parsed), nil
		}
	}
	return strings.ToLower(http.DetectContentType(prefix[:prefixBytes])), nil
}

func cleanupPresentedFiles(files []*contract.OutputFile) {
	for _, file := range files {
		file.Cleanup()
	}
}

func dshOutputFileUnavailable(runtimeID, stage string, err error) *contract.TurnError {
	slog.Warn("DSH presented file unavailable", "runtime_id", strings.TrimSpace(runtimeID), "stage", stage, "error", err)
	return &contract.TurnError{Code: contract.ErrorFileUnavailable, Message: "DSH presented file is unavailable"}
}

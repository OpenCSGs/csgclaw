package dsh

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"csgclaw/internal/agentengine/contract"
)

type acpPromptBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

var safeInputNamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func preparePromptInput(ctx context.Context, turnID contract.TurnID, workspace string, input []contract.InputPart) ([]acpPromptBlock, func(), *contract.TurnError) {
	needsFiles := false
	for _, part := range input {
		needsFiles = needsFiles || part.Kind == contract.InputPartFile
	}
	cleanup := func() {}
	var turnDir string
	var workspaceRoot *os.Root
	if needsFiles {
		workspace = strings.TrimSpace(workspace)
		if workspace == "" {
			return nil, cleanup, inputFileUnavailable("resolve_workspace", fmt.Errorf("DSH workspace is unavailable"))
		}
		var err error
		workspaceRoot, err = os.OpenRoot(workspace)
		if err != nil {
			return nil, cleanup, inputFileUnavailable("open_workspace", err)
		}
		inputRoot := filepath.Join(".csgclaw", "engine-inputs")
		if err := workspaceRoot.MkdirAll(inputRoot, 0o700); err != nil {
			_ = workspaceRoot.Close()
			return nil, cleanup, inputFileUnavailable("prepare_input_root", err)
		}
		turnDir, err = makeInputTempDir(workspaceRoot, inputRoot, safeInputName(string(turnID))+"-")
		if err != nil {
			_ = workspaceRoot.Close()
			return nil, cleanup, inputFileUnavailable("prepare_turn_directory", err)
		}
		var cleanupOnce sync.Once
		cleanup = func() {
			cleanupOnce.Do(func() {
				_ = workspaceRoot.RemoveAll(turnDir)
				_ = workspaceRoot.Close()
			})
		}
	}

	blocks := make([]acpPromptBlock, 0, len(input))
	for index, part := range input {
		switch part.Kind {
		case contract.InputPartText:
			if part.Text != "" {
				blocks = append(blocks, acpPromptBlock{Type: "text", Text: part.Text})
			}
		case contract.InputPartFile:
			if part.File == nil || part.File.Resolved == nil {
				cleanup()
				return nil, func() {}, inputFileUnavailable("resolve_input", fmt.Errorf("input file is unresolved"))
			}
			path, err := copyVerifiedInput(ctx, workspaceRoot, workspace, turnDir, index, *part.File)
			if err != nil {
				cleanup()
				return nil, func() {}, inputFileUnavailable("copy_input", err)
			}
			blocks = append(blocks, acpPromptBlock{
				Type: "text",
				Text: fmt.Sprintf("Attached file %q is available in the Runtime workspace at %s", part.File.Resolved.Name, path),
			})
		default:
			cleanup()
			return nil, func() {}, &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "unsupported DSH input part"}
		}
	}
	if len(blocks) == 0 {
		cleanup()
		return nil, func() {}, &contract.TurnError{Code: contract.ErrorInvalidRequest, Message: "DSH prompt input is required"}
	}
	return blocks, cleanup, nil
}

func inputFileUnavailable(stage string, err error) *contract.TurnError {
	slog.Warn("DSH Runtime input file unavailable", "stage", stage, "error", err)
	return &contract.TurnError{Code: contract.ErrorFileUnavailable, Message: "Runtime input file is unavailable"}
}

func copyVerifiedInput(ctx context.Context, root *os.Root, workspace, turnDir string, index int, input contract.InputFile) (string, error) {
	if input.Resolved == nil {
		return "", fmt.Errorf("input file %q is unresolved", input.ID)
	}
	source, err := input.Resolved.Open(ctx)
	if err != nil {
		return "", fmt.Errorf("open input file %q: %w", input.Resolved.Name, err)
	}
	defer source.Close()
	name := fmt.Sprintf("%03d-%s-%s", index, safeInputName(input.ID), safeInputName(filepath.Base(input.Resolved.Name)))
	destination := filepath.Join(turnDir, name)
	suffix, err := randomInputSuffix()
	if err != nil {
		return "", fmt.Errorf("create Runtime-local input name %q: %w", input.Resolved.Name, err)
	}
	temporary := destination + ".tmp-" + suffix
	target, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create Runtime-local input %q: %w", input.Resolved.Name, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(target, hash), source)
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		_ = root.Remove(temporary)
		return "", fmt.Errorf("copy input file %q: %v", input.Resolved.Name, errors.Join(copyErr, closeErr))
	}
	actualHash := fmt.Sprintf("%x", hash.Sum(nil))
	if !strings.EqualFold(actualHash, strings.TrimSpace(input.Resolved.SHA256)) {
		_ = root.Remove(temporary)
		return "", fmt.Errorf("input file %q SHA-256 does not match", input.Resolved.Name)
	}
	if err := root.Rename(temporary, destination); err != nil {
		_ = root.Remove(temporary)
		return "", fmt.Errorf("activate Runtime-local input %q: %w", input.Resolved.Name, err)
	}
	return filepath.Join(workspace, destination), nil
}

func makeInputTempDir(root *os.Root, parent, prefix string) (string, error) {
	for range 100 {
		suffix, err := randomInputSuffix()
		if err != nil {
			return "", err
		}
		name := filepath.Join(parent, prefix+suffix)
		if err := root.Mkdir(name, 0o700); err == nil {
			return name, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("create unique Runtime-local input directory")
}

func randomInputSuffix() (string, error) {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func safeInputName(value string) string {
	value = safeInputNamePattern.ReplaceAllString(strings.TrimSpace(value), "_")
	value = strings.Trim(value, "._-")
	if value == "" {
		return "input"
	}
	if len(value) > 80 {
		return value[:80]
	}
	return value
}

package codex

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type credentialFileSnapshot struct {
	exists bool
	data   []byte
	mode   os.FileMode
}

func (r *Runtime) provisionWorkspaceCredentials(ctx context.Context, workspace string, environment []string, credentials map[string]string, previous []string, initShell string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("Codex workspace is required for Runtime credentials")
	}
	if err := r.mkdirAll(workspace, 0o755); err != nil {
		return fmt.Errorf("create Codex workspace for Runtime credentials: %w", err)
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return fmt.Errorf("open Codex workspace for Runtime credentials: %w", err)
	}
	defer root.Close()

	normalized, err := normalizeCredentialFiles(credentials)
	if err != nil {
		return err
	}
	previousPaths, err := normalizeCredentialPaths(previous)
	if err != nil {
		return err
	}
	allPaths := make(map[string]struct{}, len(normalized)+len(previousPaths))
	for name := range normalized {
		allPaths[name] = struct{}{}
	}
	for _, name := range previousPaths {
		allPaths[name] = struct{}{}
	}
	ordered := make([]string, 0, len(allPaths))
	for name := range allPaths {
		ordered = append(ordered, name)
	}
	slices.Sort(ordered)

	snapshots := make(map[string]credentialFileSnapshot, len(ordered))
	for _, name := range ordered {
		snapshot, err := snapshotCredentialFile(root, name)
		if err != nil {
			return err
		}
		snapshots[name] = snapshot
	}
	rollback := func(cause error) error {
		var rollbackErrors []error
		for _, name := range ordered {
			snapshot := snapshots[name]
			if snapshot.exists {
				if err := atomicWriteCredential(root, name, snapshot.data, snapshot.mode); err != nil {
					rollbackErrors = append(rollbackErrors, fmt.Errorf("restore Runtime credential %q: %w", name, err))
				}
			} else if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove new Runtime credential %q: %w", name, err))
			}
		}
		if len(rollbackErrors) == 0 {
			return cause
		}
		return errors.Join(append([]error{cause}, rollbackErrors...)...)
	}

	for _, name := range ordered {
		content, exists := normalized[name]
		if !exists {
			if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
				return rollback(fmt.Errorf("remove replaced Runtime credential %q: %w", name, err))
			}
			continue
		}
		if err := atomicWriteCredential(root, name, []byte(content), 0o600); err != nil {
			return rollback(fmt.Errorf("write Runtime credential %q: %w", name, err))
		}
	}

	if strings.TrimSpace(initShell) == "" {
		return nil
	}
	if err := r.runInitShell(ctx, workspace, initShell, environment); err != nil {
		return rollback(err)
	}
	return nil
}

func normalizeCredentialFiles(input map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(input))
	for name, content := range input {
		normalized, err := normalizeCredentialPath(name)
		if err != nil {
			return nil, err
		}
		if _, exists := out[normalized]; exists {
			return nil, fmt.Errorf("duplicate Runtime credential path %q", normalized)
		}
		out[normalized] = content
	}
	return out, nil
}

func normalizeCredentialPaths(input []string) ([]string, error) {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, name := range input {
		normalized, err := normalizeCredentialPath(name)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	slices.Sort(out)
	return out, nil
}

func normalizeCredentialPath(name string) (string, error) {
	if strings.TrimSpace(name) == "" || name != strings.TrimSpace(name) {
		return "", fmt.Errorf("Runtime credential path must be a non-empty relative path")
	}
	if strings.ContainsRune(name, 0) || strings.Contains(name, `\`) {
		return "", fmt.Errorf("Runtime credential path %q is invalid", name)
	}
	clean := pathpkg.Clean(name)
	if clean == "." || pathpkg.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("Runtime credential path %q must remain inside the Codex workspace", name)
	}
	return filepath.FromSlash(clean), nil
}

func snapshotCredentialFile(root *os.Root, name string) (credentialFileSnapshot, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return credentialFileSnapshot{}, nil
	}
	if err != nil {
		return credentialFileSnapshot{}, fmt.Errorf("inspect Runtime credential %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return credentialFileSnapshot{}, fmt.Errorf("Runtime credential path %q must reference a regular file", name)
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return credentialFileSnapshot{}, fmt.Errorf("read existing Runtime credential %q: %w", name, err)
	}
	return credentialFileSnapshot{exists: true, data: data, mode: info.Mode().Perm()}, nil
}

func atomicWriteCredential(root *os.Root, name string, data []byte, mode os.FileMode) error {
	parent := filepath.Dir(name)
	if parent != "." {
		if err := root.MkdirAll(parent, 0o700); err != nil {
			return err
		}
	}
	token := make([]byte, 8)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	tempName := filepath.Join(parent, "."+filepath.Base(name)+".credential-"+hex.EncodeToString(token))
	temp, err := root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = root.Remove(tempName)
		}
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(mode.Perm()); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := root.Rename(tempName, name); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (r *Runtime) runInitShell(ctx context.Context, workspace, script string, environment []string) error {
	if r.deps.RunInitShell != nil {
		if err := r.deps.RunInitShell(ctx, workspace, script, append([]string(nil), environment...)); err != nil {
			return fmt.Errorf("Codex initShell failed: %w", err)
		}
		return nil
	}
	var command *exec.Cmd
	if runtime.GOOS == "windows" {
		command = exec.CommandContext(ctx, "cmd.exe", "/D", "/Q")
	} else {
		command = exec.CommandContext(ctx, "/bin/sh", "-eu")
	}
	command.Dir = workspace
	command.Env = append([]string(nil), environment...)
	command.Stdin = strings.NewReader(script)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return fmt.Errorf("Codex initShell failed: %w", err)
	}
	return nil
}

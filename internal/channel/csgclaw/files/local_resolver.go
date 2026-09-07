package files

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel"
	"csgclaw/internal/im"
)

type workspaceResolver interface {
	WorkspaceRootByID(agentID string) (string, error)
}

// LocalResolver authorizes an attachment against the routed source event and
// materializes it inside the bound Agent workspace when needed.
type LocalResolver struct {
	im         *im.Service
	workspaces workspaceResolver
	engine     agentengine.Interface
}

func NewLocalResolver(imService *im.Service, workspaces workspaceResolver, engine agentengine.Interface) (*LocalResolver, error) {
	if imService == nil {
		return nil, fmt.Errorf("IM service is required")
	}
	if workspaces == nil {
		return nil, fmt.Errorf("workspace resolver is required")
	}
	if engine == nil {
		return nil, fmt.Errorf("agent engine is required")
	}
	return &LocalResolver{im: imService, workspaces: workspaces, engine: engine}, nil
}

func (r *LocalResolver) Resolve(
	ctx context.Context,
	binding channel.Binding,
	event channel.Event,
	attachment channel.MessageAttachment,
) (agentengine.InputFile, func(), error) {
	if err := contextError(ctx); err != nil {
		return agentengine.InputFile{}, nil, err
	}
	attachmentID := strings.TrimSpace(attachment.ID)
	if attachmentID == "" || !eventContainsAttachment(event, attachmentID) {
		return agentengine.InputFile{}, nil, fmt.Errorf("attachment is not part of the routed source event")
	}
	workspaceRoot, err := r.workspaces.WorkspaceRootByID(strings.TrimSpace(binding.AgentID))
	if err != nil {
		return agentengine.InputFile{}, nil, fmt.Errorf("resolve agent workspace: %w", err)
	}
	resolved := attachment
	if strings.TrimSpace(resolved.WorkspacePath) == "" {
		relativeDir := filepath.ToSlash(filepath.Join(".csgclaw", "attachments", strings.TrimSpace(event.RoomID), strings.TrimSpace(event.MessageID)))
		resolved, err = r.im.MaterializeAttachment(attachmentID, workspaceRoot, relativeDir)
		if err != nil {
			return agentengine.InputFile{}, nil, err
		}
	}
	release := managedAttachmentRelease(workspaceRoot, resolved.WorkspacePath)
	sourcePath, err := resolveManagedAttachmentSourcePath(workspaceRoot, resolved.WorkspacePath)
	if err != nil {
		if release != nil {
			release()
		}
		return agentengine.InputFile{}, nil, fmt.Errorf("resolve materialized attachment source: %w", err)
	}
	conversation := r.engine.Conversations(strings.TrimSpace(binding.AgentID))
	if conversation == nil || conversation.Files() == nil {
		if release != nil {
			release()
		}
		return agentengine.InputFile{}, nil, fmt.Errorf("agent file store is unavailable")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		if release != nil {
			release()
		}
		return agentengine.InputFile{}, nil, fmt.Errorf("open materialized attachment: %w", err)
	}
	info, statErr := source.Stat()
	if statErr != nil {
		_ = source.Close()
		if release != nil {
			release()
		}
		return agentengine.InputFile{}, nil, fmt.Errorf("inspect materialized attachment: %w", statErr)
	}
	created, createErr := conversation.Files().Create(ctx, agentengine.FileCreateRequest{
		Name:      resolved.Name,
		MIMEType:  resolved.MediaType,
		SizeBytes: info.Size(),
		SHA256:    resolved.SHA256,
	}, source)
	closeErr := source.Close()
	if createErr != nil || closeErr != nil {
		if release != nil {
			release()
		}
		return agentengine.InputFile{}, nil, fmt.Errorf("store materialized attachment: %w", errors.Join(createErr, closeErr))
	}
	if strings.TrimSpace(created.ID) == "" {
		if release != nil {
			release()
		}
		return agentengine.InputFile{}, nil, fmt.Errorf("stored attachment file id is empty")
	}
	files := conversation.Files()
	var releaseOnce sync.Once
	return agentengine.InputFile{ID: created.ID}, func() {
		releaseOnce.Do(func() {
			if err := files.Delete(context.Background(), created.ID); err != nil && agentengine.ErrorCodeOf(err) != agentengine.ErrorFileNotFound {
				slog.Warn("delete staged IM attachment from Agent Engine failed", "agent_file_id", created.ID, "error", err)
			}
			if release != nil {
				release()
			}
		})
	}, nil
}

func (r *LocalResolver) ResolveContext(
	ctx context.Context,
	binding channel.Binding,
	event channel.Event,
) (channel.Event, func(), error) {
	if event.ThreadContext == nil || len(event.ThreadContext.Context) == 0 {
		return event, nil, nil
	}
	if err := contextError(ctx); err != nil {
		return channel.Event{}, nil, err
	}
	workspaceRoot, err := r.workspaces.WorkspaceRootByID(strings.TrimSpace(binding.AgentID))
	if err != nil {
		return channel.Event{}, nil, fmt.Errorf("resolve agent workspace for thread context: %w", err)
	}
	thread := *event.ThreadContext
	thread.Context = append([]channel.ThreadContextMessage(nil), event.ThreadContext.Context...)
	var releases []func()
	releaseAll := func() {
		for index := len(releases) - 1; index >= 0; index-- {
			if releases[index] != nil {
				releases[index]()
			}
		}
	}
	for messageIndex := range thread.Context {
		message := &thread.Context[messageIndex]
		message.Attachments = append([]channel.MessageAttachment(nil), message.Attachments...)
		for attachmentIndex := range message.Attachments {
			attachment := &message.Attachments[attachmentIndex]
			if release := managedAttachmentRelease(workspaceRoot, attachment.WorkspacePath); release != nil {
				releases = append(releases, release)
			}
			attachmentID := strings.TrimSpace(attachment.ID)
			if attachmentID == "" {
				continue
			}
			relativeDir := filepath.ToSlash(filepath.Join(
				".csgclaw", "attachments",
				strings.TrimSpace(event.RoomID),
				strings.TrimSpace(event.MessageID),
				"thread-context",
				strings.TrimSpace(message.ID),
			))
			resolved, resolveErr := r.im.MaterializeAttachment(attachmentID, workspaceRoot, relativeDir)
			if resolveErr != nil {
				releaseAll()
				return channel.Event{}, nil, fmt.Errorf("materialize thread context attachment %q: %w", attachmentID, resolveErr)
			}
			*attachment = resolved
			releases = append(releases, managedAttachmentRelease(workspaceRoot, resolved.WorkspacePath))
		}
	}
	event.ThreadContext = &thread
	if len(releases) == 0 {
		return event, nil, nil
	}
	return event, releaseAll, nil
}

// resolveManagedAttachmentSourcePath converts the API-facing workspace path
// into a host path that Agent Engine can read without depending on the server
// process working directory. The resolved file must remain under the managed
// attachment directory, including when workspace path components are symlinks.
func resolveManagedAttachmentSourcePath(workspaceRoot, sourcePath string) (string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	sourcePath = strings.TrimSpace(filepath.FromSlash(sourcePath))
	if workspaceRoot == "" || sourcePath == "" {
		return "", fmt.Errorf("workspace root and attachment path are required")
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root symlinks: %w", err)
	}
	target := sourcePath
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve attachment path: %w", err)
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("resolve attachment path symlinks: %w", err)
	}
	managedRoot := filepath.Join(root, ".csgclaw", "attachments")
	managedRoot, err = filepath.EvalSymlinks(managedRoot)
	if err != nil {
		return "", fmt.Errorf("resolve managed attachment root: %w", err)
	}
	if !pathWithinRoot(managedRoot, target) || target == managedRoot {
		return "", fmt.Errorf("attachment path is outside the managed workspace directory")
	}
	return target, nil
}

func pathWithinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func managedAttachmentRelease(workspaceRoot, sourcePath string) func() {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	sourcePath = strings.TrimSpace(filepath.FromSlash(sourcePath))
	if workspaceRoot == "" || sourcePath == "" {
		return nil
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil
	}
	target := sourcePath
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return nil
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	managedRoot := filepath.Join(root, ".csgclaw", "attachments")
	managedRelative, err := filepath.Rel(managedRoot, target)
	if err != nil || managedRelative == "." || managedRelative == ".." || strings.HasPrefix(managedRelative, ".."+string(filepath.Separator)) {
		return nil
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				slog.Warn("remove materialized IM attachment failed", "path", target, "error", err)
				return
			}
			for parent := filepath.Dir(target); parent != managedRoot; parent = filepath.Dir(parent) {
				if err := os.Remove(parent); err != nil {
					break
				}
			}
		})
	}
}

func eventContainsAttachment(event channel.Event, attachmentID string) bool {
	for _, candidate := range event.Attachments {
		if strings.TrimSpace(candidate.ID) == attachmentID {
			return true
		}
	}
	return false
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

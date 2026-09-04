package agentengine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"csgclaw/internal/activity"
	"csgclaw/internal/agent"
	"csgclaw/internal/runtime"
	"csgclaw/internal/runtime/codex"
)

type agentServiceAdapter struct {
	service *agent.Service
}

// New wires the existing Agent Service through private Engine facades.
func New(service *agent.Service) *Engine {
	adapter := agentServiceAdapter{service: service}
	files := NewFileStore()
	return &Engine{agents: agentFacade{service: service, files: files}, runtimes: adapter, files: files}
}

func (a agentServiceAdapter) conversationRuntime(ctx context.Context, agentID string) (conversationRuntimeAdapter, func(), *TurnError) {
	if a.service == nil {
		return nil, nil, &TurnError{Code: ErrorAgentUnavailable, Message: "agent service is required"}
	}
	lease, err := a.service.AcquireExecution(ctx, agentID)
	if err != nil {
		return nil, nil, &TurnError{Code: ErrorAgentUnavailable, Message: err.Error()}
	}
	release := lease.Release
	selected := lease.Agent
	if !strings.EqualFold(strings.TrimSpace(selected.Status), string(runtime.StateRunning)) || strings.TrimSpace(selected.RuntimeID) == "" {
		release()
		return nil, nil, &TurnError{Code: ErrorAgentUnavailable, Message: fmt.Sprintf("agent %q is unavailable", agentID)}
	}
	if !strings.EqualFold(strings.TrimSpace(selected.RuntimeKind), agent.RuntimeKindCodex) {
		release()
		return nil, nil, &TurnError{Code: ErrorRuntimeAdapterUnavailable, Message: fmt.Sprintf("runtime adapter %q is unavailable", selected.RuntimeKind)}
	}
	codexRuntime, ok := lease.Runtime.(codexConversationRuntime)
	if !ok {
		release()
		return nil, nil, &TurnError{Code: ErrorRuntimeAdapterUnavailable, Message: "Codex runtime does not support direct conversations"}
	}
	return &codexRuntimeAdapter{runtimeID: selected.RuntimeID, runtime: codexRuntime}, release, nil
}

type codexConversationRuntime interface {
	EnsureEngineSession(ctx context.Context, runtimeID, conversationKey string) (string, error)
	ExistingEngineSession(ctx context.Context, runtimeID, conversationKey string) (string, bool, error)
	PromptTurn(ctx context.Context, runtimeID, sessionID, turnID string, blocks []codex.PromptContentBlock, accepted func()) error
	SubscribeSession(runtimeID, sessionID string) (<-chan activity.RuntimeEvent, func())
	ResetConversation(ctx context.Context, runtimeID, conversationKey string) error
	WorkspaceDir(runtimeID string) (string, error)
	PermissionBroker() codex.PermissionBroker
	UserInputBroker() codex.UserInputBroker
}

type directUserInputResponder interface {
	RespondDirect(context.Context, string, string, activity.RequestUserInputResponse) (activity.UserInputSnapshot, error)
}

type codexRuntimeAdapter struct {
	runtimeID string
	runtime   codexConversationRuntime
}

func (a *codexRuntimeAdapter) Run(ctx context.Context, request TurnRequest, sink EventSink) TurnResult {
	blocks, cleanup, inputErr := a.prepareInput(ctx, request.ID, request.Input)
	if inputErr != nil {
		return TurnResult{Status: TurnFailed, Error: inputErr}
	}
	defer cleanup()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sessionID, sessionErr := a.session(runCtx, request)
	if sessionErr != nil {
		return TurnResult{Status: TurnFailed, Error: sessionErr}
	}
	events, unsubscribe := a.runtime.SubscribeSession(a.runtimeID, sessionID)
	defer unsubscribe()

	accepted := make(chan struct{})
	var dispatchedState atomic.Bool
	promptDone := make(chan error, 1)
	go func() {
		promptDone <- a.runtime.PromptTurn(runCtx, a.runtimeID, sessionID, string(request.ID), blocks, func() {
			if dispatchedState.CompareAndSwap(false, true) {
				close(accepted)
			}
		})
	}()

	dispatched := false
	promptReturned := false
	runtimeDone := false
	eventStreamClosed := false
	var output strings.Builder
	var files []*OutputFile
	filesOwned := true
	defer func() {
		if filesOwned {
			cleanupOutputFiles(files)
		}
	}()
	stopPrompt := func(result TurnResult) TurnResult {
		cancel()
		if !promptReturned {
			<-promptDone
			promptReturned = true
		}
		result.Dispatched = dispatchedState.Load()
		return result
	}
	for {
		if runtimeDone && promptReturned {
			if eventStreamClosed {
				result := failedResult(ErrorRuntimeFailed, "Runtime event stream closed before prompt completion")
				result.Dispatched = dispatched
				return result
			}
			filesOwned = false
			return TurnResult{Status: TurnSucceeded, Output: output.String(), Dispatched: dispatched, files: files}
		}
		select {
		case <-accepted:
			dispatched = dispatchedState.Load()
			accepted = nil
		case <-runCtx.Done():
			return stopPrompt(resultFromContext(runCtx, runCtx.Err()))
		case err := <-promptDone:
			promptReturned = true
			dispatched = dispatchedState.Load()
			if err != nil {
				result := resultFromContext(runCtx, err)
				result.Dispatched = dispatched
				return result
			}
		case event, ok := <-events:
			if !ok {
				runtimeDone = true
				eventStreamClosed = true
				events = nil
				continue
			}
			if strings.TrimSpace(event.RuntimeID) != strings.TrimSpace(a.runtimeID) || strings.TrimSpace(event.SessionID) != strings.TrimSpace(sessionID) {
				continue
			}
			if result := a.handleEvent(runCtx, request, sink, event, &output, &files); result != nil {
				return stopPrompt(*result)
			}
			if event.Kind == activity.RuntimeEventPromptCompleted {
				runtimeDone = true
			}
		}
	}
}

func (a *codexRuntimeAdapter) session(ctx context.Context, request TurnRequest) (string, *TurnError) {
	if request.Continuation == ContinuationRequireExisting {
		sessionID, ok, err := a.runtime.ExistingEngineSession(ctx, a.runtimeID, string(request.ConversationKey))
		if err != nil {
			return "", &TurnError{Code: ErrorRuntimeFailed, Message: err.Error()}
		}
		if !ok {
			return "", &TurnError{Code: ErrorConversationNotResumable, Message: "conversation has no Runtime-native mapping"}
		}
		return sessionID, nil
	}
	sessionID, err := a.runtime.EnsureEngineSession(ctx, a.runtimeID, string(request.ConversationKey))
	if err != nil {
		return "", &TurnError{Code: ErrorRuntimeFailed, Message: fmt.Sprintf("ensure Codex conversation: %v", err)}
	}
	return sessionID, nil
}

func (a *codexRuntimeAdapter) handleEvent(ctx context.Context, request TurnRequest, sink EventSink, event activity.RuntimeEvent, output *strings.Builder, files *[]*OutputFile) *TurnResult {
	emit := func(turnEvent TurnEvent) *TurnResult {
		if err := emitTurnEvent(ctx, sink, turnEvent); err != nil {
			result := resultFromContext(ctx, err)
			return &result
		}
		return nil
	}
	switch event.Kind {
	case activity.RuntimeEventPromptFailed:
		message := strings.TrimSpace(event.Error)
		if message == "" {
			message = "agent turn ended without a final response"
		}
		result := failedResult(ErrorRuntimeFailed, message)
		return &result
	case activity.RuntimeEventActionRequest:
		return a.handleInteraction(ctx, request.Interaction, permissionInteraction(event), sink)
	case activity.RuntimeEventUserInputRequest:
		return a.handleInteraction(ctx, request.Interaction, userInputInteraction(event), sink)
	case activity.RuntimeEventStructuredOutput:
		artifact, ok := event.Payload.(activity.StructuredOutputArtifact)
		if !ok {
			result := failedResult(ErrorRuntimeFailed, "Codex returned invalid structured output")
			return &result
		}
		if artifact.RequestUserInput != nil {
			if result := emit(TurnEvent{Kind: TurnEventOutputItem, Text: event.Text, Output: &OutputItem{Kind: OutputItemRequestUserInput, Payload: *artifact.RequestUserInput}}); result != nil {
				return result
			}
		}
		for _, link := range artifact.ResourceLinks {
			linkCopy := link
			if result := emit(TurnEvent{Kind: TurnEventOutputItem, Output: &OutputItem{Kind: OutputItemResourceLink, Payload: linkCopy}}); result != nil {
				return result
			}
		}
	case activity.RuntimeEventFileOutput:
		runtimeFile, ok := event.Payload.(activity.RuntimeFile)
		if !ok {
			result := failedResult(ErrorRuntimeFailed, "Codex returned invalid file output")
			return &result
		}
		file, fileErr := a.authorizeOutputFile(ctx, runtimeFile)
		if fileErr != nil {
			result := TurnResult{Status: TurnFailed, Error: fileErr}
			return &result
		}
		*files = append(*files, file)
		return nil
	case activity.RuntimeEventTextDelta:
		phase := runtimeEventPhase(event)
		if phase != "" && phase != "final_answer" {
			return nil
		}
		if result := emit(TurnEvent{Kind: TurnEventTextDelta, Text: event.Text}); result != nil {
			return result
		}
		_, _ = output.WriteString(event.Text)
	case activity.RuntimeEventThoughtDelta:
		return emit(TurnEvent{Kind: TurnEventThoughtDelta, Thought: event.Text})
	case activity.RuntimeEventToolCallStart:
		return emit(TurnEvent{Kind: TurnEventToolCallStart, Tool: toolActivity(event)})
	case activity.RuntimeEventToolCallUpdate:
		return emit(TurnEvent{Kind: TurnEventToolCallUpdate, Tool: toolActivity(event)})
	case activity.RuntimeEventPlanUpdate, activity.RuntimeEventActionDecision, activity.RuntimeEventUserInputResolved:
		return emit(TurnEvent{Kind: TurnEventActivityUpdate, Activity: &ActivityUpdate{
			ID: event.ActionID + event.UserInputID, Kind: string(event.Kind), Status: event.ActionStatus + event.UserInputStatus, Payload: event.Payload,
		}})
	}
	return nil
}

func (a *codexRuntimeAdapter) authorizeOutputFile(ctx context.Context, request activity.RuntimeFile) (*OutputFile, *TurnError) {
	workspace, err := a.runtime.WorkspaceDir(a.runtimeID)
	if err != nil {
		return nil, a.runtimeFileUnavailable("resolve_workspace", err)
	}
	workspace = strings.TrimSpace(workspace)
	relativePath, err := cleanRuntimeOutputPath(request.Path)
	if err != nil {
		return nil, &TurnError{Code: ErrorFileUnavailable, Message: err.Error()}
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, a.runtimeFileUnavailable("open_workspace", err)
	}
	source, info, err := openRuntimeOutputSource(root, relativePath)
	if err != nil {
		_ = root.Close()
		return nil, a.runtimeFileUnavailable("open_source", err)
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = filepath.Base(relativePath)
	}
	file, snapshotErr := newOutputFile(ctx, OutputFileMetadata{Name: name, MediaType: "application/octet-stream", SizeBytes: info.Size()}, source)
	closeErr := source.Close()
	rootCloseErr := root.Close()
	if snapshotErr != nil || closeErr != nil || rootCloseErr != nil {
		if file != nil {
			file.cleanup()
		}
		return nil, a.runtimeFileUnavailable("snapshot_source", errors.Join(snapshotErr, closeErr, rootCloseErr))
	}
	download, err := file.open(ctx)
	if err != nil {
		file.cleanup()
		return nil, a.runtimeFileUnavailable("verify_snapshot", err)
	}
	prefix := make([]byte, 512)
	prefixBytes, readErr := io.ReadFull(contextReader{ctx: ctx, reader: download}, prefix)
	downloadCloseErr := download.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) || downloadCloseErr != nil {
		file.cleanup()
		return nil, a.runtimeFileUnavailable("read_snapshot_prefix", errors.Join(readErr, downloadCloseErr))
	}
	mediaType, err := runtimeOutputMediaType(request.MIMEType, name, prefix[:prefixBytes])
	if err != nil {
		file.cleanup()
		return nil, &TurnError{Code: ErrorFileUnavailable, Message: err.Error()}
	}
	file.MediaType = mediaType
	return file, nil
}

func (a *codexRuntimeAdapter) runtimeFileUnavailable(stage string, err error) *TurnError {
	slog.Warn("Runtime output file unavailable", "runtime_id", strings.TrimSpace(a.runtimeID), "stage", stage, "error", err)
	return &TurnError{Code: ErrorFileUnavailable, Message: "Runtime output file is unavailable"}
}

func cleanRuntimeOutputPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	cleaned := filepath.Clean(path)
	if path == "" || cleaned == "." || filepath.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Runtime output path must stay within the Runtime workspace")
	}
	return cleaned, nil
}

func openRuntimeOutputSource(root *os.Root, relativePath string) (*os.File, os.FileInfo, error) {
	info, err := root.Lstat(relativePath)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect Runtime output file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("Runtime output must be a regular non-symlink file")
	}
	file, err := root.Open(relativePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open Runtime output file: %w", err)
	}
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return nil, nil, fmt.Errorf("Runtime output file changed before it could be opened")
	}
	return file, openedInfo, nil
}

func runtimeOutputMediaType(requested, name string, prefix []byte) (string, error) {
	detected := http.DetectContentType(prefix)
	mediaType := strings.TrimSpace(requested)
	if mediaType != "" {
		parsed, _, err := mime.ParseMediaType(mediaType)
		if err != nil || strings.TrimSpace(parsed) == "" {
			return "", fmt.Errorf("Runtime output MIME type is invalid")
		}
		mediaType = strings.ToLower(parsed)
	} else if byExtension := mime.TypeByExtension(filepath.Ext(name)); byExtension != "" {
		parsed, _, _ := mime.ParseMediaType(byExtension)
		mediaType = strings.ToLower(parsed)
	} else {
		mediaType = strings.ToLower(detected)
	}
	if strings.HasPrefix(mediaType, "image/") && !strings.HasPrefix(strings.ToLower(detected), "image/") {
		return "", fmt.Errorf("Runtime output content does not match image MIME type %q", mediaType)
	}
	return mediaType, nil
}

func (a *codexRuntimeAdapter) handleInteraction(ctx context.Context, policy InteractionPolicy, interaction InteractionRequest, sink EventSink) *TurnResult {
	switch policy {
	case InteractionReject:
		result := failedResult(ErrorInteractionUnsupported, "Runtime interaction is not supported by this caller")
		return &result
	case InteractionSkipUserInput:
		resolution := InteractionResolution{InteractionID: interaction.ID, ResponderID: "agent-engine"}
		if interaction.Kind == InteractionPermission {
			snapshot, _ := interaction.Payload.(activity.ActivitySnapshot)
			for _, option := range snapshot.Options {
				kind := strings.ToLower(strings.TrimSpace(option.Kind))
				if !strings.Contains(kind, "allow") && !strings.Contains(kind, "accept") {
					resolution.OptionID = option.ID
					break
				}
			}
		}
		if err := a.Resolve(ctx, interaction, resolution); err != nil {
			result := failedResult(ErrorInteractionUnsupported, err.Message)
			return &result
		}
		return nil
	default:
		if err := emitTurnEvent(ctx, sink, TurnEvent{Kind: TurnEventInteractionRequest, Interaction: &interaction}); err != nil {
			result := resultFromContext(ctx, err)
			return &result
		}
		return nil
	}
}

func (a *codexRuntimeAdapter) Reset(ctx context.Context, key ConversationKey) *TurnError {
	if err := a.runtime.ResetConversation(ctx, a.runtimeID, string(key)); err != nil {
		return &TurnError{Code: ErrorRuntimeFailed, Message: err.Error()}
	}
	return nil
}

func (a *codexRuntimeAdapter) Resolve(ctx context.Context, request InteractionRequest, resolution InteractionResolution) *TurnError {
	switch request.Kind {
	case InteractionPermission:
		if strings.TrimSpace(resolution.OptionID) == "" {
			return &TurnError{Code: ErrorInvalidRequest, Message: "permission option ID is required"}
		}
		if _, err := a.runtime.PermissionBroker().Decide(ctx, request.ID, resolution.OptionID); err != nil {
			return &TurnError{Code: ErrorInteractionNotFound, Message: err.Error()}
		}
	case InteractionUserInput:
		responder, ok := a.runtime.UserInputBroker().(directUserInputResponder)
		if !ok {
			return &TurnError{Code: ErrorInteractionUnsupported, Message: "Codex user-input broker does not support Engine resolution"}
		}
		answers := make(map[string]activity.RequestUserInputAnswer, len(resolution.Answers))
		for questionID, answer := range resolution.Answers {
			if answer.Skipped {
				continue
			}
			answers[questionID] = activity.RequestUserInputAnswer{Answers: append([]string(nil), answer.Values...)}
		}
		responderID := strings.TrimSpace(resolution.ResponderID)
		if responderID == "" {
			responderID = "agent-engine"
		}
		if _, err := responder.RespondDirect(ctx, request.ID, responderID, activity.RequestUserInputResponse{Answers: answers}); err != nil {
			return &TurnError{Code: ErrorInteractionNotFound, Message: err.Error()}
		}
	default:
		return &TurnError{Code: ErrorInteractionUnsupported, Message: "interaction kind is unsupported"}
	}
	return nil
}

func permissionInteraction(event activity.RuntimeEvent) InteractionRequest {
	snapshot, _ := event.Payload.(activity.ActivitySnapshot)
	id := strings.TrimSpace(event.ActionID)
	if id == "" {
		id = snapshot.ID
	}
	return InteractionRequest{ID: id, Kind: InteractionPermission, Title: snapshot.Title, Payload: snapshot}
}

func userInputInteraction(event activity.RuntimeEvent) InteractionRequest {
	snapshot, _ := event.Payload.(activity.UserInputSnapshot)
	id := strings.TrimSpace(event.UserInputID)
	if id == "" {
		id = snapshot.ID
	}
	return InteractionRequest{ID: id, Kind: InteractionUserInput, Title: "User input required", Payload: snapshot}
}

func toolActivity(event activity.RuntimeEvent) *ToolActivity {
	return &ToolActivity{
		ID: event.ToolCallID, Kind: event.ToolKind, Title: event.ToolTitle, Status: event.ToolStatus,
		InputSummary: event.ToolInputSummary, OutputSummary: event.ToolOutputSummary, Payload: event.Payload,
	}
}

func runtimeEventPhase(event activity.RuntimeEvent) string {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return ""
	}
	phase, _ := payload["phase"].(string)
	return strings.TrimSpace(strings.ToLower(phase))
}

var safeInputNamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (a *codexRuntimeAdapter) prepareInput(ctx context.Context, turnID TurnID, input []InputPart) ([]codex.PromptContentBlock, func(), *TurnError) {
	needsFiles := false
	for _, part := range input {
		needsFiles = needsFiles || part.Kind == InputPartFile
	}
	cleanup := func() {}
	var turnDir string
	var workspaceRoot *os.Root
	var workspace string
	if needsFiles {
		var err error
		workspace, err = a.runtime.WorkspaceDir(a.runtimeID)
		if err != nil {
			return nil, cleanup, a.runtimeInputFileUnavailable("resolve_workspace", err)
		}
		workspaceRoot, err = os.OpenRoot(workspace)
		if err != nil {
			return nil, cleanup, a.runtimeInputFileUnavailable("open_workspace", err)
		}
		inputRoot := filepath.Join(".csgclaw", "engine-inputs")
		if err := workspaceRoot.MkdirAll(inputRoot, 0o700); err != nil {
			_ = workspaceRoot.Close()
			return nil, cleanup, a.runtimeInputFileUnavailable("prepare_input_root", err)
		}
		turnDir, err = makeRootTempDir(workspaceRoot, inputRoot, safeInputName(string(turnID))+"-")
		if err != nil {
			_ = workspaceRoot.Close()
			return nil, cleanup, a.runtimeInputFileUnavailable("prepare_turn_directory", err)
		}
		var cleanupOnce sync.Once
		cleanup = func() {
			cleanupOnce.Do(func() {
				_ = workspaceRoot.RemoveAll(turnDir)
				_ = workspaceRoot.Close()
			})
		}
	}
	blocks := make([]codex.PromptContentBlock, 0, len(input))
	for index, part := range input {
		if part.Kind == InputPartText {
			blocks = append(blocks, codex.TextBlock(part.Text))
			continue
		}
		path, err := copyVerifiedInput(ctx, workspaceRoot, workspace, turnDir, index, *part.File)
		if err != nil {
			cleanup()
			return nil, func() {}, a.runtimeInputFileUnavailable("copy_input", err)
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(part.File.file.MediaType)), "image/") {
			blocks = append(blocks, codex.LocalImageBlock(path))
		} else {
			blocks = append(blocks, codex.TextBlock(fmt.Sprintf("Attached file %q is available in the Runtime workspace at %s", part.File.file.Name, path)))
		}
	}
	return blocks, cleanup, nil
}

func (a *codexRuntimeAdapter) runtimeInputFileUnavailable(stage string, err error) *TurnError {
	slog.Warn("Runtime input file unavailable", "runtime_id", strings.TrimSpace(a.runtimeID), "stage", stage, "error", err)
	return &TurnError{Code: ErrorFileUnavailable, Message: "Runtime input file is unavailable"}
}

func copyVerifiedInput(ctx context.Context, root *os.Root, workspace, turnDir string, index int, input InputFile) (string, error) {
	if input.file == nil {
		return "", fmt.Errorf("input file %q is unresolved", input.ID)
	}
	source, err := input.file.open(ctx)
	if err != nil {
		return "", fmt.Errorf("open input file %q: %w", input.file.Name, err)
	}
	defer source.Close()
	name := fmt.Sprintf("%03d-%s-%s", index, safeInputName(input.ID), safeInputName(filepath.Base(input.file.Name)))
	destination := filepath.Join(turnDir, name)
	suffix, err := randomInputSuffix()
	if err != nil {
		return "", fmt.Errorf("create Runtime-local input name %q: %w", input.file.Name, err)
	}
	temporary := destination + ".tmp-" + suffix
	target, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create Runtime-local input %q: %w", input.file.Name, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(target, hash), source)
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		_ = root.Remove(temporary)
		return "", fmt.Errorf("copy input file %q: %v", input.file.Name, errors.Join(copyErr, closeErr))
	}
	actualHash := fmt.Sprintf("%x", hash.Sum(nil))
	if !strings.EqualFold(actualHash, strings.TrimSpace(input.file.SHA256)) {
		_ = root.Remove(temporary)
		return "", fmt.Errorf("input file %q SHA-256 does not match", input.file.Name)
	}
	if err := root.Rename(temporary, destination); err != nil {
		_ = root.Remove(temporary)
		return "", fmt.Errorf("activate Runtime-local input %q: %w", input.file.Name, err)
	}
	return filepath.Join(workspace, destination), nil
}

func makeRootTempDir(root *os.Root, parent, prefix string) (string, error) {
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

func emitTurnEvent(ctx context.Context, sink EventSink, event TurnEvent) error {
	if sink == nil {
		return nil
	}
	return sink.Emit(ctx, event)
}

func resultFromContext(ctx context.Context, err error) TurnResult {
	if err == nil {
		err = ctx.Err()
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return TurnResult{Status: TurnCanceled, Error: &TurnError{Code: ErrorCanceled, Message: err.Error()}}
	}
	return failedResult(ErrorRuntimeFailed, err.Error())
}

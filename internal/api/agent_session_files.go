package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/im"
)

type agentSessionInput struct {
	userInput     bool
	attachmentIDs []string
	Text          string
	Files         []agentSessionInputFile
}

type agentSessionInputFile struct {
	Name         string
	MediaType    string
	SizeBytes    int64
	encodedBytes int
	open         func() (io.ReadCloser, error)
}

func parseAgentSessionFile(name, data string) (agentSessionInputFile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return agentSessionInputFile{}, fmt.Errorf("input_file filename is required")
	}
	if _, err := im.ValidateAttachmentName(name); err != nil {
		return agentSessionInputFile{}, err
	}
	value, ok := strings.CutPrefix(data, "data:")
	header, encoded, found := strings.Cut(value, ",")
	mediaType, encodedHeader := strings.CutSuffix(header, ";base64")
	if !ok || !found || !encodedHeader {
		return agentSessionInputFile{}, fmt.Errorf("input_file file_data must be a Base64 data URL")
	}
	mediaType, _, err := mime.ParseMediaType(mediaType)
	if err != nil || mediaType == "" {
		return agentSessionInputFile{}, fmt.Errorf("input_file file_data must include a valid media type")
	}
	reader := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(encoded))
	size, err := io.Copy(io.Discard, io.LimitReader(reader, im.MaxAttachmentFileBytes+1))
	if err != nil {
		return agentSessionInputFile{}, fmt.Errorf("invalid input_file Base64 data: %w", err)
	}
	if size > im.MaxAttachmentFileBytes {
		return agentSessionInputFile{}, fmt.Errorf("%w: input_file exceeds %d bytes", errAttachmentPayloadTooLarge, im.MaxAttachmentFileBytes)
	}
	if size == 0 {
		return agentSessionInputFile{}, fmt.Errorf("input_file must not be empty")
	}
	return agentSessionInputFile{
		Name: name, MediaType: mediaType, SizeBytes: size, encodedBytes: len(encoded),
		open: func() (io.ReadCloser, error) {
			return io.NopCloser(base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(encoded))), nil
		},
	}, nil
}

func parseAgentSessionMultipart(w http.ResponseWriter, r *http.Request) (agentSessionInput, bool, error) {
	// Reserve space for the input fields and multipart framing.
	r.Body = http.MaxBytesReader(w, r.Body, im.MaxAttachmentMessageBytes+2*agentSessionResponseBodyLimit)
	if err := r.ParseMultipartForm(multipartAttachmentMemory); err != nil {
		return agentSessionInput{}, false, fmt.Errorf("decode multipart request: %w", err)
	}
	form := r.MultipartForm
	files, err := sessionMultipartFiles(form, "file")
	if err != nil {
		return agentSessionInput{}, false, err
	}
	inputs := form.Value["input"]
	for name := range form.Value {
		if name != "input" && name != "stream" && name != "user_input" {
			return agentSessionInput{}, false, fmt.Errorf("unknown multipart field %q", name)
		}
	}
	if len(inputs) != 1 {
		return agentSessionInput{}, false, fmt.Errorf("expected one input field")
	}
	metadataBytes := len(inputs[0])
	var request agentSessionResponseRequest
	for _, field := range []string{"stream", "user_input"} {
		values := form.Value[field]
		if len(values) == 0 {
			continue
		}
		if len(values) > 1 {
			return agentSessionInput{}, false, fmt.Errorf("expected at most one %s field", field)
		}
		metadataBytes += len(values[0])
		enabled, err := strconv.ParseBool(values[0])
		if err != nil {
			return agentSessionInput{}, false, fmt.Errorf("%s must be a boolean", field)
		}
		if field == "stream" {
			request.Stream = &enabled
		} else {
			request.UserInput = enabled
		}
	}
	if metadataBytes > agentSessionResponseBodyLimit {
		return agentSessionInput{}, false, fmt.Errorf("%w: multipart fields exceed %d bytes", errAttachmentPayloadTooLarge, agentSessionResponseBodyLimit)
	}
	input := strings.Trim(inputs[0], " \t\n\r")
	if input == "" || input == "null" {
		return agentSessionInput{}, false, fmt.Errorf("input must be a JSON string or an array of user messages")
	}
	request.Input = json.RawMessage(input)
	return request.parseInput(files, int64(metadataBytes))
}

func sessionMultipartFiles(form *multipart.Form, field string) ([]agentSessionInputFile, error) {
	if len(form.File) != 1 || len(form.File[field]) == 0 {
		return nil, fmt.Errorf("expected one or more %s parts", field)
	}
	uploads, err := readMultipartAttachmentUploads(form)
	if err != nil {
		return nil, err
	}
	files := make([]agentSessionInputFile, 0, len(uploads))
	for _, upload := range uploads {
		name, err := im.ValidateAttachmentName(upload.Name)
		if err != nil {
			return nil, err
		}
		if upload.SizeBytes == 0 {
			return nil, fmt.Errorf("file %q must not be empty", upload.Name)
		}
		mediaType := upload.MediaType
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		files = append(files, agentSessionInputFile{
			Name: name, MediaType: mediaType, SizeBytes: upload.SizeBytes, open: upload.Open,
		})
	}
	return files, nil
}

func (input agentSessionInput) validateFileLimits(existing []im.MessageAttachment) error {
	if len(input.Files)+len(existing) > im.MaxAttachmentsPerMessage {
		return fmt.Errorf("too many input files: max %d", im.MaxAttachmentsPerMessage)
	}
	var total, largest int64
	for _, file := range existing {
		size := file.SizeBytes
		largest = max(largest, size)
		total += size
	}
	for _, file := range input.Files {
		largest = max(largest, file.SizeBytes)
		total += file.SizeBytes
	}
	if largest > im.MaxAttachmentFileBytes {
		return fmt.Errorf("%w: input_file exceeds %d bytes", errAttachmentPayloadTooLarge, im.MaxAttachmentFileBytes)
	}
	if total > im.MaxAttachmentMessageBytes {
		return fmt.Errorf("%w: input files exceed %d bytes per request", errAttachmentPayloadTooLarge, im.MaxAttachmentMessageBytes)
	}
	return nil
}

func sessionTurnInput(text string, attachments []im.MessageAttachment) []agentengine.InputPart {
	parts := make([]agentengine.InputPart, 0, len(attachments)+1)
	if text != "" {
		parts = append(parts, agentengine.InputPart{Kind: agentengine.InputPartText, Text: text})
	}
	for _, att := range attachments {
		parts = append(parts, agentengine.InputPart{
			Kind:          agentengine.InputPartWorkspaceFile,
			WorkspaceFile: &agentengine.WorkspaceFile{Path: att.WorkspacePath, Name: att.Name, MIMEType: att.MediaType},
		})
	}
	return parts
}

func (h *Handler) storeSessionUploads(ctx context.Context, agent agentengine.Agent, sessionID, batchID string, files []agentSessionInputFile) ([]im.MessageAttachment, error) {
	if len(files) == 0 {
		return nil, nil
	}
	sources := make([]im.AttachmentSource, 0, len(files))
	for _, file := range files {
		reader, err := file.open()
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		sources = append(sources, im.AttachmentSource{Name: file.Name, MediaType: file.MediaType, SizeBytes: file.SizeBytes, Reader: reader})
	}
	return h.saveSessionAttachmentSources(ctx, agent, sessionID, batchID, sources)
}

func (h *Handler) storeSessionOutputFiles(
	ctx context.Context, conversation agentengine.ConversationInterface, agent agentengine.Agent, sessionID, responseID string,
	files []agentengine.OutputFile,
) ([]im.MessageAttachment, []string, error) {
	if len(files) == 0 {
		return nil, nil, nil
	}
	sources, rejected := delivery.OpenOutputFileSources(ctx, conversation.Files(), files)
	defer delivery.CloseOutputFileSources(sources)
	if len(sources) == 0 {
		return nil, rejected, ctx.Err()
	}
	attachments, err := h.saveSessionAttachmentSources(ctx, agent, sessionID, responseID, sources)
	return attachments, rejected, err
}

func (h *Handler) saveSessionAttachmentSources(ctx context.Context, agent agentengine.Agent, sessionID, batchID string, sources []im.AttachmentSource) ([]im.MessageAttachment, error) {
	workspaceRoot, err := h.sessionWorkspaceRoot(agent)
	if err != nil {
		return nil, err
	}
	return h.im.StoreSessionResponseAttachments(ctx, agent.ID, sessionID, batchID, workspaceRoot, sources)
}

func (h *Handler) sessionWorkspaceRoot(agent agentengine.Agent) (string, error) {
	root, err := h.configuredAgentWorkspace(agent.ID, agent.Spec.Runtime.Options)
	if err != nil {
		return "", err
	}
	return filepath.Abs(root)
}

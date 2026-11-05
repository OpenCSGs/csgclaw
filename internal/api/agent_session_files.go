package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/channel/csgclaw/delivery"
	"csgclaw/internal/im"
)

type agentSessionInput struct {
	attachmentIDs []string
	Text          string
	Files         []agentSessionInputFile
}

type agentSessionInputFile struct {
	Name         string
	MediaType    string
	SizeBytes    int64
	sha256       string
	encodedBytes int
	open         func() (io.ReadCloser, error)
}

func parseAgentSessionFile(name, data string) (agentSessionInputFile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return agentSessionInputFile{}, fmt.Errorf("input_file filename is required")
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
	// Reserve space for both the JSON payload and multipart framing.
	r.Body = http.MaxBytesReader(w, r.Body, im.MaxAttachmentMessageBytes+2*agentSessionResponseBodyLimit)
	if err := r.ParseMultipartForm(multipartAttachmentMemory); err != nil {
		return agentSessionInput{}, false, fmt.Errorf("decode multipart request: %w", err)
	}
	form := r.MultipartForm
	if len(form.Value) != 1 || len(form.Value["payload"]) != 1 {
		return agentSessionInput{}, false, fmt.Errorf("expected one payload field")
	}
	payload := form.Value["payload"][0]
	if len(payload) > agentSessionResponseBodyLimit {
		return agentSessionInput{}, false, fmt.Errorf("%w: payload exceeds %d bytes", errAttachmentPayloadTooLarge, agentSessionResponseBodyLimit)
	}
	files, err := sessionMultipartFiles(form)
	if err != nil {
		return agentSessionInput{}, false, err
	}
	return decodeAgentSessionResponseRequest(strings.NewReader(payload), files)
}

func sessionMultipartFiles(form *multipart.Form) ([]agentSessionInputFile, error) {
	if len(form.File) != 1 || len(form.File["files"]) == 0 {
		return nil, fmt.Errorf("expected one or more files parts")
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

func (input agentSessionInput) validateFileLimits() error {
	if len(input.Files) > im.MaxAttachmentsPerMessage {
		return fmt.Errorf("too many input files: max %d", im.MaxAttachmentsPerMessage)
	}
	var total int64
	for _, file := range input.Files {
		if file.SizeBytes > im.MaxAttachmentFileBytes {
			return fmt.Errorf("%w: input_file exceeds %d bytes", errAttachmentPayloadTooLarge, im.MaxAttachmentFileBytes)
		}
		total += file.SizeBytes
	}
	if total > im.MaxAttachmentMessageBytes {
		return fmt.Errorf("%w: input files exceed %d bytes per request", errAttachmentPayloadTooLarge, im.MaxAttachmentMessageBytes)
	}
	return nil
}

type preparedSessionInput struct {
	parts []agentengine.InputPart
	files []agentengine.OutputFile
}

func (input agentSessionInput) prepare(ctx context.Context, files agentengine.FileInterface) (preparedSessionInput, func(), error) {
	var prepared preparedSessionInput
	cleanup := func() {
		for _, file := range prepared.files {
			if err := files.Delete(context.Background(), file.ID); err != nil {
				slog.Warn("delete session input file failed", "file_id", file.ID, "error", err)
			}
		}
	}
	prepared.parts = make([]agentengine.InputPart, 0, len(input.Files)+1)
	if input.Text != "" {
		prepared.parts = append(prepared.parts, agentengine.InputPart{Kind: agentengine.InputPartText, Text: input.Text})
	}
	for _, file := range input.Files {
		reader, err := file.open()
		if err != nil {
			return prepared, cleanup, err
		}
		created, err := files.Create(ctx, agentengine.FileCreateRequest{
			Name: file.Name, MIMEType: file.MediaType, SizeBytes: file.SizeBytes, SHA256: file.sha256,
		}, reader)
		_ = reader.Close()
		if err != nil {
			return prepared, cleanup, err
		}
		prepared.files = append(prepared.files, created)
		prepared.parts = append(prepared.parts, agentengine.InputPart{Kind: agentengine.InputPartFile, File: &agentengine.InputFile{ID: created.ID}})
	}
	return prepared, cleanup, nil
}

func (h *Handler) storeSessionFiles(
	ctx context.Context, store agentengine.FileInterface, agent agentengine.Agent, sessionID, responseID string,
	files []agentengine.OutputFile,
) ([]im.MessageAttachment, error) {
	if len(files) == 0 {
		return nil, nil
	}
	sources, rejected := delivery.OpenOutputFileSources(ctx, store, files)
	defer delivery.CloseOutputFileSources(sources)
	if len(rejected) > 0 {
		return nil, fmt.Errorf("session attachment content is unavailable")
	}
	return h.saveSessionAttachmentSources(agent, sessionID, responseID, sources)
}

func (h *Handler) saveSessionAttachmentSources(agent agentengine.Agent, sessionID, batchID string, sources []im.AttachmentSource) ([]im.MessageAttachment, error) {
	workspaceRoot, err := h.configuredAgentWorkspace(agent.ID, agent.Spec.Runtime.Options)
	if err != nil {
		return nil, err
	}
	workspaceRoot, err = filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return h.im.StoreSessionResponseAttachments(agent.ID, sessionID, batchID, workspaceRoot, sources)
}

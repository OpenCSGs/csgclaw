package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"strings"

	"csgclaw/internal/agentengine"
	"csgclaw/internal/im"
)

type agentSessionInput struct {
	Text  string
	Files []agentSessionInputFile
}

type agentSessionInputFile struct {
	Name      string
	MediaType string
	Data      string
	SizeBytes int64
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
	return agentSessionInputFile{Name: name, MediaType: mediaType, Data: encoded, SizeBytes: size}, nil
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

func (input agentSessionInput) prepare(ctx context.Context, files agentengine.FileInterface) ([]agentengine.InputPart, func(), error) {
	var ids []string
	cleanup := func() {
		for _, id := range ids {
			if err := files.Delete(context.Background(), id); err != nil {
				slog.Warn("delete session input file failed", "file_id", id, "error", err)
			}
		}
	}
	parts := make([]agentengine.InputPart, 0, len(input.Files)+1)
	if input.Text != "" {
		parts = append(parts, agentengine.InputPart{Kind: agentengine.InputPartText, Text: input.Text})
	}
	for _, file := range input.Files {
		reader := base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(file.Data))
		created, err := files.Create(ctx, agentengine.FileCreateRequest{
			Name: file.Name, MIMEType: file.MediaType, SizeBytes: file.SizeBytes,
		}, reader)
		if err != nil {
			return nil, cleanup, err
		}
		ids = append(ids, created.ID)
		parts = append(parts, agentengine.InputPart{Kind: agentengine.InputPartFile, File: &agentengine.InputFile{ID: created.ID}})
	}
	return parts, cleanup, nil
}

package agentengine

import (
	"context"
	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/diagnostics"
)

func captureTurnInputFiles(ctx context.Context, request TurnRequest) {
	for _, part := range request.Input {
		if part.File != nil && part.File.Resolved != nil {
			captureTurnOutputFile(ctx, part.File.Resolved)
		}
	}
}
func captureTurnOutputFile(ctx context.Context, file *contract.OutputFile) {
	record := diagnostics.From(ctx)
	if record == nil || file == nil {
		return
	}
	if !diagnostics.TextMediaType(file.MediaType) {
		record.Capture("execution.file", map[string]any{"metadata": file.Metadata(), "content_status": "binary_metadata_only"})
		return
	}
	content, err := file.Open(ctx)
	if err != nil {
		record.Capture("execution.file", map[string]any{"metadata": file.Metadata(), "content_status": "unavailable"})
		record.Issue("file_content_unavailable")
		return
	}
	defer content.Close()
	record.CaptureTextFile(file.Metadata(), content)
}

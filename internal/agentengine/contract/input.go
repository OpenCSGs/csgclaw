package contract

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// WorkspaceFile identifies an attachment already published in the worker's workspace.
// Only trusted in-process callers can set it after authorizing the attachment.
type WorkspaceFile struct {
	Path     string
	Name     string
	MIMEType string
}

// Valid checks that the reference names a file in the active workspace.
func (f *WorkspaceFile) Valid() bool {
	return f != nil && strings.TrimSpace(f.Name) != "" && f.Path != "." &&
		strings.TrimSpace(f.Path) == f.Path && fs.ValidPath(f.Path) &&
		filepath.IsLocal(filepath.FromSlash(f.Path)) && !strings.ContainsAny(f.Path, `\:`)
}

// CloneInput detaches pointer fields for turn admission and replay.
func CloneInput(parts []InputPart) []InputPart {
	result := append([]InputPart(nil), parts...)
	for i := range result {
		if f := result[i].File; f != nil {
			copy := *f
			result[i].File = &copy
		}
		if f := result[i].WorkspaceFile; f != nil {
			copy := *f
			result[i].WorkspaceFile = &copy
		}
	}
	return result
}

// ValidateInput checks the normalized turn input before dispatch.
func ValidateInput(input []InputPart) *TurnError {
	for _, part := range input {
		switch part.Kind {
		case InputPartText:
			if part.File != nil || part.WorkspaceFile != nil || strings.TrimSpace(part.Text) == "" {
				return &TurnError{Code: ErrorInvalidRequest, Message: "text input must contain only non-empty text"}
			}
		case InputPartFile:
			if part.File == nil || part.WorkspaceFile != nil || strings.TrimSpace(part.Text) != "" {
				return &TurnError{Code: ErrorInvalidRequest, Message: "file input must include only a file"}
			}
			if strings.TrimSpace(part.File.ID) == "" {
				return &TurnError{Code: ErrorInvalidRequest, Message: "file ID is required"}
			}
		case InputPartWorkspaceFile:
			if part.File != nil || part.Text != "" || !part.WorkspaceFile.Valid() {
				return &TurnError{Code: ErrorInvalidRequest, Message: "workspace input requires an authorized relative file path"}
			}
		default:
			return &TurnError{Code: ErrorInvalidRequest, Message: fmt.Sprintf("unsupported input kind %q", part.Kind)}
		}
	}
	return nil
}

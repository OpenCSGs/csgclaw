package api

import (
	"archive/zip"
	"encoding/json"
	"io"
	"time"
)

// The small ZIP comment lets the browser show collection status without
// inflating or parsing the potentially very large diagnostic JSON entry.
type diagnosticZIPMetadata struct {
	SchemaVersion int    `json:"schema_version"`
	TurnID        string `json:"turn_id"`
	Complete      bool   `json:"complete"`
}

func writeDiagnosticZIP(out io.Writer, filename, turnID string, render func(io.Writer) (bool, error)) error {
	archive := zip.NewWriter(out)
	entry, err := archive.CreateHeader(&zip.FileHeader{Name: filename, Method: zip.Deflate, Modified: time.Now().UTC()})
	if err != nil {
		return err
	}
	complete, err := render(entry)
	if err != nil {
		// A failed export must not acquire a valid central directory. The client
		// rejects the incomplete archive and keeps the dialog available to retry.
		return err
	}
	metadata, err := json.Marshal(diagnosticZIPMetadata{SchemaVersion: 1, TurnID: turnID, Complete: complete})
	if err != nil {
		return err
	}
	if err := archive.SetComment(string(metadata)); err != nil {
		return err
	}
	return archive.Close()
}

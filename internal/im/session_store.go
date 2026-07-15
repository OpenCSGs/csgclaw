package im

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Keep each jsonl line safely under bufio.Scanner's default 64KiB token limit.
	maxSessionJSONLLineBytes = 56 * 1024
	// Upper bound when reading a line (legacy oversized rows or manual edits).
	maxSessionReadLineBytes = 16 * 1024 * 1024
	sessionBlobsDirName     = "blobs"
)

// sessionMessageLine is the on-disk jsonl shape. blob_ref points at spillover payload.
type sessionMessageLine struct {
	ID          string              `json:"id"`
	SenderID    string              `json:"sender_id"`
	Kind        string              `json:"kind,omitempty"`
	Content     string              `json:"content"`
	Event       *EventPayload       `json:"event,omitempty"`
	Metadata    map[string]any      `json:"metadata,omitempty"`
	CreatedAt   string              `json:"created_at"`
	Mentions    []Mention           `json:"mentions"`
	RelatesTo   *MessageRelation    `json:"relates_to,omitempty"`
	Thread      *ThreadSummary      `json:"thread,omitempty"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
	BlobRef     string              `json:"blob_ref,omitempty"`
}

type sessionMessageBlob struct {
	Content     string              `json:"content,omitempty"`
	Event       *EventPayload       `json:"event,omitempty"`
	Thread      *ThreadSummary      `json:"thread,omitempty"`
	Attachments []MessageAttachment `json:"attachments,omitempty"`
}

func messageToSessionLine(message Message) sessionMessageLine {
	return sessionMessageLine{
		ID:          message.ID,
		SenderID:    message.SenderID,
		Kind:        message.Kind,
		Content:     message.Content,
		Event:       message.Event,
		Metadata:    message.Metadata,
		CreatedAt:   message.CreatedAt.UTC().Format(timeRFC3339Nano),
		Mentions:    message.Mentions,
		RelatesTo:   message.RelatesTo,
		Thread:      message.Thread,
		Attachments: message.Attachments,
	}
}

func sessionLineToMessage(line sessionMessageLine) (Message, error) {
	createdAt, err := parseSessionCreatedAt(line.CreatedAt)
	if err != nil {
		return Message{}, err
	}
	return Message{
		ID:          line.ID,
		SenderID:    line.SenderID,
		Kind:        line.Kind,
		Content:     line.Content,
		Event:       line.Event,
		Metadata:    line.Metadata,
		CreatedAt:   createdAt,
		Mentions:    line.Mentions,
		RelatesTo:   line.RelatesTo,
		Thread:      line.Thread,
		Attachments: cloneMessageAttachments(line.Attachments),
	}, nil
}

const timeRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

func parseSessionCreatedAt(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("missing created_at")
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, raw)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("parse created_at %q: %w", raw, err)
	}
	return parsed.UTC(), nil
}

func validateSessionPathSegment(segment string) error {
	segment = strings.TrimSpace(segment)
	switch {
	case segment == "":
		return fmt.Errorf("empty session path segment")
	case segment == "." || segment == "..":
		return fmt.Errorf("invalid session path segment %q", segment)
	case strings.Contains(segment, "/"), strings.Contains(segment, "\\"):
		return fmt.Errorf("invalid session path segment %q", segment)
	default:
		return nil
	}
}

func sessionBlobRelativePath(roomID, messageID string) (string, error) {
	if err := validateSessionPathSegment(roomID); err != nil {
		return "", err
	}
	if err := validateSessionPathSegment(messageID); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(sessionBlobsDirName, roomID, messageID+".json")), nil
}

func loadMessagesJSONL(path, roomID string) ([]Message, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open im session: %w", err)
	}
	defer file.Close()

	lines, err := readSessionJSONLLines(file)
	if err != nil {
		return nil, err
	}

	sessionsRoot := filepath.Dir(path)
	messages := make([]Message, 0, len(lines))
	for _, line := range lines {
		message, err := decodeSessionMessageLine(sessionsRoot, line)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func readSessionJSONLLines(r io.Reader) ([][]byte, error) {
	reader := bufio.NewReader(r)
	var lines [][]byte
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				if err == io.EOF {
					break
				}
				continue
			}
			if len(trimmed) > maxSessionReadLineBytes {
				return nil, fmt.Errorf("im session line exceeds %d bytes", maxSessionReadLineBytes)
			}
			lines = append(lines, bytes.Clone(trimmed))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read im session: %w", err)
		}
	}
	return lines, nil
}

func decodeSessionMessageLine(sessionsRoot string, line []byte) (Message, error) {
	var record sessionMessageLine
	if err := json.Unmarshal(line, &record); err != nil {
		// Legacy rows are plain Message JSON without blob_ref.
		var legacy Message
		if legacyErr := json.Unmarshal(line, &legacy); legacyErr != nil {
			return Message{}, fmt.Errorf("decode im session line: %w", err)
		}
		return legacy, nil
	}

	message, err := sessionLineToMessage(record)
	if err != nil {
		return Message{}, fmt.Errorf("decode im session line: %w", err)
	}
	if strings.TrimSpace(record.BlobRef) == "" {
		return message, nil
	}

	blob, err := loadSessionMessageBlob(sessionsRoot, record.BlobRef)
	if err != nil {
		return Message{}, err
	}
	message.Content = blob.Content
	message.Event = blob.Event
	message.Thread = blob.Thread
	message.Attachments = cloneMessageAttachments(blob.Attachments)
	return message, nil
}

func loadSessionMessageBlob(sessionsRoot, relativeRef string) (sessionMessageBlob, error) {
	relativeRef = strings.TrimSpace(relativeRef)
	if relativeRef == "" {
		return sessionMessageBlob{}, fmt.Errorf("empty blob_ref")
	}
	if filepath.IsAbs(relativeRef) {
		return sessionMessageBlob{}, fmt.Errorf("blob_ref must be relative")
	}
	path := filepath.Join(sessionsRoot, filepath.FromSlash(relativeRef))
	sessionsRootClean := filepath.Clean(sessionsRoot)
	pathClean := filepath.Clean(path)
	if !strings.HasPrefix(pathClean, sessionsRootClean+string(os.PathSeparator)) && pathClean != sessionsRootClean {
		return sessionMessageBlob{}, fmt.Errorf("blob_ref escapes sessions dir")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return sessionMessageBlob{}, fmt.Errorf("read im session blob: %w", err)
	}
	var blob sessionMessageBlob
	if err := json.Unmarshal(data, &blob); err != nil {
		return sessionMessageBlob{}, fmt.Errorf("decode im session blob: %w", err)
	}
	return blob, nil
}

func saveMessagesJSONL(path, roomID string, messages []Message) error {
	if err := validateSessionPathSegment(roomID); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create im session dir: %w", err)
	}

	sessionsRoot := filepath.Dir(path)
	if len(messages) == 0 {
		if err := truncateSessionJSONL(path); err != nil {
			return err
		}
		return removeRoomSessionBlobs(sessionsRoot, roomID)
	}

	blobDir := filepath.Join(sessionsRoot, sessionBlobsDirName, roomID)
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		return fmt.Errorf("create im session blobs dir: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create im session: %w", err)
	}
	defer file.Close()

	keepBlobs := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		line, blobRef, err := encodeSessionMessageLine(sessionsRoot, roomID, message)
		if err != nil {
			return err
		}
		if blobRef != "" {
			keepBlobs[filepath.Base(blobRef)] = struct{}{}
		}
		if _, err := file.Write(line); err != nil {
			return fmt.Errorf("write im session: %w", err)
		}
		if _, err := io.WriteString(file, "\n"); err != nil {
			return fmt.Errorf("write im session newline: %w", err)
		}
	}

	if err := cleanupRoomSessionBlobs(blobDir, keepBlobs); err != nil {
		return err
	}
	return nil
}

func truncateSessionJSONL(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create im session: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close im session: %w", err)
	}
	return nil
}

func encodeSessionMessageLine(sessionsRoot, roomID string, message Message) ([]byte, string, error) {
	if err := validateSessionPathSegment(message.ID); err != nil {
		return nil, "", fmt.Errorf("encode im session message: %w", err)
	}

	line := messageToSessionLine(message)
	data, err := json.Marshal(line)
	if err != nil {
		return nil, "", fmt.Errorf("encode im session message: %w", err)
	}
	if len(data) <= maxSessionJSONLLineBytes {
		return data, "", nil
	}

	relativeRef, err := sessionBlobRelativePath(roomID, message.ID)
	if err != nil {
		return nil, "", err
	}
	blob := sessionMessageBlob{
		Content:     message.Content,
		Event:       message.Event,
		Thread:      message.Thread,
		Attachments: message.Attachments,
	}
	blobData, err := json.Marshal(blob)
	if err != nil {
		return nil, "", fmt.Errorf("encode im session blob: %w", err)
	}
	blobPath := filepath.Join(sessionsRoot, filepath.FromSlash(relativeRef))
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		return nil, "", fmt.Errorf("create im session blob dir: %w", err)
	}
	if err := os.WriteFile(blobPath, blobData, 0o600); err != nil {
		return nil, "", fmt.Errorf("write im session blob: %w", err)
	}

	line.Content = ""
	line.Event = nil
	line.Thread = nil
	line.Attachments = nil
	line.BlobRef = relativeRef
	data, err = json.Marshal(line)
	if err != nil {
		return nil, "", fmt.Errorf("encode im session message: %w", err)
	}
	if len(data) > maxSessionJSONLLineBytes {
		return nil, "", fmt.Errorf("encode im session message: metadata exceeds %d bytes for message %s", maxSessionJSONLLineBytes, message.ID)
	}
	return data, relativeRef, nil
}

func cleanupRoomSessionBlobs(blobDir string, keep map[string]struct{}) error {
	entries, err := os.ReadDir(blobDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read im session blobs dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if _, ok := keep[entry.Name()]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(blobDir, entry.Name())); err != nil {
			return fmt.Errorf("remove stale im session blob: %w", err)
		}
	}
	return nil
}

func removeRoomSessionBlobs(sessionsDir, roomID string) error {
	if err := validateSessionPathSegment(roomID); err != nil {
		return err
	}
	blobDir := filepath.Join(sessionsDir, sessionBlobsDirName, roomID)
	if err := os.RemoveAll(blobDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove im session blobs dir: %w", err)
	}
	return nil
}

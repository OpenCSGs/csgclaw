package im

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/diagnostics"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	assetsDirName                    = "assets"
	assetObjectsDirName              = "objects"
	assetBlobsDirName                = "blobs"
	attachmentKindFile               = "file"
	attachmentKindImage              = "image"
	MaxAttachmentsPerMessage         = 10
	MaxAttachmentFileBytes           = apitypes.MaxAttachmentFileBytes
	MaxAttachmentMessageBytes  int64 = 2 * 1024 * 1024 * 1024
	maxSafeAttachmentNameBytes       = 160
)

// AttachmentSource is an internal, streamed generated file. The caller owns Reader.
// User uploads continue to use MessageAttachmentUpload and its upload quotas.
type AttachmentSource struct {
	Name      string
	MediaType string
	SizeBytes int64
	SHA256    string
	Reader    io.ReadCloser
}

type attachmentObject struct {
	ID             string    `json:"id"`
	BlobSHA256     string    `json:"blob_sha256,omitempty"`
	OriginalName   string    `json:"original_name"`
	SafeName       string    `json:"safe_name"`
	MediaType      string    `json:"media_type"`
	Kind           string    `json:"kind"`
	SizeBytes      int64     `json:"size_bytes"`
	SHA256         string    `json:"sha256"`
	CreatedAt      time.Time `json:"created_at"`
	CreatedBy      string    `json:"created_by"`
	RoomID         string    `json:"room_id"`
	SessionID      string    `json:"session_id,omitempty"`
	MessageID      string    `json:"message_id"`
	Width          int       `json:"width,omitempty"`
	Height         int       `json:"height,omitempty"`
	DownloadToken  string    `json:"download_token"`
	WorkspacePath  string    `json:"workspace_path,omitempty"`
	WorkspaceRoot  string    `json:"workspace_root,omitempty"`
	SessionBatchID string    `json:"session_batch_id,omitempty"`
}

type AttachmentFile struct {
	Attachment    MessageAttachment
	Path          string
	SafeName      string
	MediaType     string
	DownloadToken string
	workspaceRoot string
	workspacePath string
}

// Prepared files live outside the object/blob directories so a concurrent
// message save or room cleanup cannot collect them before they are committed.
type preparedAttachment struct {
	path        string
	object      attachmentObject
	corruptBlob os.FileInfo
}

func discardPreparedAttachments(files []preparedAttachment) {
	for _, file := range files {
		_ = os.Remove(file.path)
	}
}

func (s *Service) prepareMessageAttachments(uploads []MessageAttachmentUpload, sources ...AttachmentSource) ([]preparedAttachment, error) {
	if len(uploads) == 0 && len(sources) == 0 {
		return nil, nil
	}
	if s == nil || strings.TrimSpace(s.statePath) == "" {
		return nil, fmt.Errorf("attachment storage requires persistent IM state")
	}
	if len(uploads) > MaxAttachmentsPerMessage {
		return nil, fmt.Errorf("too many attachments: max %d", MaxAttachmentsPerMessage)
	}
	total := int64(0)
	for _, upload := range uploads {
		size := int64(len(upload.Data))
		if upload.Open != nil {
			size = upload.SizeBytes
		}
		if size <= 0 {
			return nil, fmt.Errorf("attachment %q is empty", strings.TrimSpace(upload.Name))
		}
		if size > MaxAttachmentFileBytes {
			return nil, fmt.Errorf("attachment %q exceeds %d bytes", strings.TrimSpace(upload.Name), MaxAttachmentFileBytes)
		}
		total += size
		if total > MaxAttachmentMessageBytes {
			return nil, fmt.Errorf("attachments exceed %d bytes per message", MaxAttachmentMessageBytes)
		}
		if _, err := ValidateAttachmentName(upload.Name); err != nil {
			return nil, err
		}
	}

	for _, upload := range uploads {
		source := AttachmentSource{Name: upload.Name, MediaType: upload.MediaType, SizeBytes: int64(len(upload.Data)), Reader: io.NopCloser(bytes.NewReader(upload.Data))}
		if upload.Open != nil {
			reader, err := upload.Open()
			if err != nil {
				return nil, fmt.Errorf("open attachment %q: %w", upload.Name, err)
			}
			if reader == nil {
				return nil, fmt.Errorf("attachment %q reader is missing", upload.Name)
			}
			defer reader.Close()
			source.Reader = reader
			source.SizeBytes = upload.SizeBytes
		}
		sources = append(sources, source)
	}
	return roomAttachmentStore(s.statePath).prepare(context.Background(), sources)
}

func (store attachmentStore) prepare(ctx context.Context, sources []AttachmentSource) ([]preparedAttachment, error) {
	attachments := make([]preparedAttachment, 0, len(sources))
	for _, source := range sources {
		att, err := store.prepareAttachment(ctx, source)
		if err != nil {
			discardPreparedAttachments(attachments)
			return nil, err
		}
		attachments = append(attachments, att)
	}
	return attachments, nil
}

// Commit metadata and blob names while holding the IM lock. All copying,
// hashing and syncing of file contents has already completed outside the lock.
func (s *Service) storeMessageAttachmentsLocked(roomID, messageID, senderID string, files []preparedAttachment) ([]MessageAttachment, error) {
	if len(files) == 0 {
		return nil, nil
	}
	attachments, err := roomAttachmentStore(s.statePath).commit(roomID, messageID, senderID, files)
	if err != nil {
		return nil, errors.Join(err, cleanupAssetFilesForState(s.statePath, s.bootstrapLocked().Rooms))
	}
	return attachments, nil
}

func (store attachmentStore) commit(roomID, messageID, senderID string, files []preparedAttachment) ([]MessageAttachment, error) {
	attachments := make([]MessageAttachment, 0, len(files))
	for _, file := range files {
		attachment, err := store.storePreparedAttachment(roomID, messageID, senderID, file)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func (store attachmentStore) prepareAttachment(ctx context.Context, source AttachmentSource) (preparedAttachment, error) {
	originalName, err := ValidateAttachmentName(source.Name)
	if err != nil {
		return preparedAttachment{}, err
	}
	if source.Reader == nil || source.SizeBytes < 0 {
		return preparedAttachment{}, fmt.Errorf("invalid attachment source for %q", source.Name)
	}
	dir := string(store)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return preparedAttachment{}, err
	}
	staged, err := os.CreateTemp(dir, ".attachment-*")
	if err != nil {
		return preparedAttachment{}, err
	}
	keep := false
	defer func() {
		_ = staged.Close()
		if !keep {
			_ = os.Remove(staged.Name())
		}
	}()
	object, err := writeAttachmentData(ctx, staged, source, originalName)
	if err != nil {
		return preparedAttachment{}, err
	}
	object.BlobSHA256 = object.SHA256
	corruptBlob, err := inspectExistingAttachmentBlob(ctx, store.blobPath(object.SHA256), object.SHA256, object.SizeBytes)
	if err != nil {
		return preparedAttachment{}, err
	}
	keep = true
	return preparedAttachment{path: staged.Name(), corruptBlob: corruptBlob, object: object}, nil
}

// writeAttachmentData streams the source once and derives its metadata from the
// same file that will be published in the room store or the session workspace.
func writeAttachmentData(ctx context.Context, staged *os.File, source AttachmentSource, originalName string) (attachmentObject, error) {
	hash := sha256.New()
	reader := attachmentReader(ctx, source.Reader)
	written, err := io.CopyN(io.MultiWriter(staged, hash), reader, source.SizeBytes)
	if err != nil {
		return attachmentObject{}, fmt.Errorf("read attachment: %w", err)
	}
	var extra [1]byte
	n, err := io.ReadFull(reader, extra[:])
	if ctx.Err() != nil {
		return attachmentObject{}, ctx.Err()
	}
	if n != 0 || err != io.EOF || written != source.SizeBytes {
		return attachmentObject{}, fmt.Errorf("attachment size changed")
	}
	sha := hex.EncodeToString(hash.Sum(nil))
	if source.SHA256 != "" && source.SHA256 != sha {
		return attachmentObject{}, fmt.Errorf("attachment checksum changed")
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return attachmentObject{}, err
	}
	header := make([]byte, 512)
	n, err = staged.Read(header)
	if err != nil && err != io.EOF {
		return attachmentObject{}, err
	}
	mediaType := normalizedAttachmentMediaType(source.MediaType, header[:n])
	kind := attachmentKindForMediaType(mediaType)
	var width, height int
	if strings.HasPrefix(mediaType, "image/") {
		if _, err := staged.Seek(0, io.SeekStart); err != nil {
			return attachmentObject{}, err
		}
		if config, _, err := image.DecodeConfig(staged); err == nil {
			width, height = config.Width, config.Height
		}
	}
	if err := staged.Sync(); err != nil {
		return attachmentObject{}, err
	}
	if err := staged.Close(); err != nil {
		return attachmentObject{}, err
	}
	id, err := newAttachmentID(sha)
	if err != nil {
		return attachmentObject{}, err
	}
	return attachmentObject{ID: id, OriginalName: originalName, SafeName: safeAttachmentName(originalName),
		MediaType: mediaType, Kind: kind, SizeBytes: written, SHA256: sha, Width: width, Height: height}, nil
}

// Blob contents are immutable once installed. Detect pre-existing damage outside
// the IM lock, while allowing another pending upload to install the same hash.
func inspectExistingAttachmentBlob(ctx context.Context, path, sha string, size int64) (os.FileInfo, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() != size {
		return info, nil
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, attachmentReader(ctx, file)); err != nil {
		return nil, err
	}
	if hex.EncodeToString(hash.Sum(nil)) != sha {
		return info, nil
	}
	return nil, nil
}

func (store attachmentStore) storePreparedAttachment(roomID, messageID, senderID string, prepared preparedAttachment) (MessageAttachment, error) {
	object := prepared.object
	blobPath := store.blobPath(object.SHA256)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o700); err != nil {
		return MessageAttachment{}, err
	}
	info, err := os.Stat(blobPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return MessageAttachment{}, err
	}
	corrupt := err == nil && prepared.corruptBlob != nil && os.SameFile(info, prepared.corruptBlob) && info.ModTime().Equal(prepared.corruptBlob.ModTime()) && info.Size() == prepared.corruptBlob.Size()
	// Reuse a valid blob, including one committed by another concurrent upload.
	// This also avoids replacing a blob that is open for download on Windows.
	if err != nil || info.Size() != object.SizeBytes || corrupt {
		if err := os.Rename(prepared.path, blobPath); err != nil {
			return MessageAttachment{}, fmt.Errorf("store attachment blob: %w", err)
		}
	}
	object.DownloadToken, err = newAttachmentDownloadToken()
	if err != nil {
		return MessageAttachment{}, err
	}
	object.CreatedAt = time.Now().UTC()
	object.CreatedBy = strings.TrimSpace(senderID)
	object.RoomID = strings.TrimSpace(roomID)
	object.MessageID = strings.TrimSpace(messageID)
	if err := writeAttachmentObject(store.objectPath(object.ID), object); err != nil {
		return MessageAttachment{}, err
	}
	return attachmentFromObject(object), nil
}

func (s *Service) AttachmentFile(id string) (AttachmentFile, error) {
	if s == nil || strings.TrimSpace(s.statePath) == "" {
		return AttachmentFile{}, fmt.Errorf("attachment storage is not configured")
	}
	for _, store := range []attachmentStore{sessionAttachmentStore(s.statePath), roomAttachmentStore(s.statePath)} {
		object, err := store.readObject(id)
		if errors.Is(err, errAttachmentNotFound) {
			continue
		}
		if err != nil {
			return AttachmentFile{}, err
		}
		if object.RoomID == "" {
			return sessionFile(object)
		}
		return store.blobFile(object)
	}
	return AttachmentFile{}, errAttachmentNotFound
}

var errAttachmentNotFound = errors.New("attachment not found")

func (store attachmentStore) readObject(id string) (attachmentObject, error) {
	id = strings.TrimSpace(id)
	if !validAttachmentID(id) {
		return attachmentObject{}, fmt.Errorf("attachment id is required")
	}
	object, err := readAttachmentObject(store.objectPath(id))
	if err != nil {
		return attachmentObject{}, err
	}
	if object.ID != id {
		return attachmentObject{}, errAttachmentNotFound
	}
	if object.SessionBatchID != "" {
		// This index can become visible while the publisher is still writing
		// the batch. Only its first index commits the whole set.
		first, err := readAttachmentObject(store.objectPath(object.SessionBatchID))
		if err != nil {
			return attachmentObject{}, err
		}
		if first.ID != object.SessionBatchID || first.SessionBatchID != "" || first.CreatedBy != object.CreatedBy ||
			first.SessionID != object.SessionID || first.MessageID != object.MessageID || first.WorkspaceRoot != object.WorkspaceRoot {
			return attachmentObject{}, errAttachmentNotFound
		}
	}
	return object, nil
}

func (store attachmentStore) blobFile(object attachmentObject) (AttachmentFile, error) {
	path := store.blobPath(object.BlobSHA256)
	if _, err := os.Stat(path); err != nil {
		return AttachmentFile{}, fmt.Errorf("stat attachment blob: %w", err)
	}
	return AttachmentFile{
		Attachment:    attachmentFromObject(object),
		Path:          path,
		SafeName:      object.SafeName,
		MediaType:     object.MediaType,
		DownloadToken: object.DownloadToken,
	}, nil
}

func (f AttachmentFile) AuthorizesDownloadToken(token string) bool {
	want := strings.TrimSpace(f.DownloadToken)
	got := strings.TrimSpace(token)
	if want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *Service) MaterializeAttachment(id, workspaceRoot, relativeDir string) (MessageAttachment, error) {
	file, err := s.AttachmentFile(id)
	if err != nil {
		return MessageAttachment{}, err
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return MessageAttachment{}, fmt.Errorf("workspace root is required")
	}
	relativeDir = strings.Trim(strings.TrimSpace(filepath.ToSlash(relativeDir)), "/")
	if relativeDir == "" {
		relativeDir = ".csgclaw/attachments"
	}
	relativeName := filepath.ToSlash(filepath.Join(relativeDir, file.Attachment.ID+"-"+file.SafeName))
	targetPath, err := prepareWorkspaceAttachmentPath(workspaceRoot, relativeName)
	if err != nil {
		return MessageAttachment{}, err
	}
	source, err := file.Open()
	if err != nil {
		return MessageAttachment{}, fmt.Errorf("read attachment blob: %w", err)
	}
	defer source.Close()
	if err := atomicWriteReader(targetPath, source, 0o600); err != nil {
		return MessageAttachment{}, fmt.Errorf("write attachment workspace file: %w", err)
	}
	att := file.Attachment
	att.WorkspacePath = relativeName
	return att, nil
}

func attachmentFromObject(object attachmentObject) MessageAttachment {
	att := MessageAttachment{
		ID:            object.ID,
		Name:          object.SafeName,
		Kind:          object.Kind,
		MediaType:     object.MediaType,
		SizeBytes:     object.SizeBytes,
		SHA256:        object.SHA256,
		CreatedAt:     object.CreatedAt,
		DownloadURL:   attachmentDownloadURL(object),
		WorkspacePath: object.WorkspacePath,
		Width:         object.Width,
		Height:        object.Height,
	}
	if att.Kind == attachmentKindImage {
		att.PreviewURL = att.DownloadURL
	}
	return att
}

func attachmentDownloadURL(object attachmentObject) string {
	downloadURL := "/api/v1/attachments/" + url.PathEscape(object.ID)
	if token := strings.TrimSpace(object.DownloadToken); token != "" {
		downloadURL += "?token=" + url.QueryEscape(token)
	}
	return downloadURL
}

func normalizedAttachmentMediaType(declared string, data []byte) string {
	detected := "application/octet-stream"
	if len(data) > 0 {
		detected = strings.ToLower(strings.TrimSpace(strings.SplitN(http.DetectContentType(data), ";", 2)[0]))
	}
	declared = strings.TrimSpace(declared)
	if declared != "" {
		if mediaType, _, err := mime.ParseMediaType(declared); err == nil && strings.TrimSpace(mediaType) != "" {
			declared = strings.ToLower(strings.TrimSpace(mediaType))
			if strings.HasPrefix(declared, "image/") {
				if strings.HasPrefix(detected, "image/") {
					return detected
				}
				if declared == "image/svg+xml" && looksLikeSVG(data) {
					return declared
				}
				return detected
			}
			return declared
		}
	}
	return detected
}

func looksLikeSVG(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	prefix := bytes.ToLower(bytes.TrimSpace(data))
	if len(prefix) > 1024 {
		prefix = prefix[:1024]
	}
	return bytes.Contains(prefix, []byte("<svg"))
}

func attachmentKindForMediaType(mediaType string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "image/") {
		return attachmentKindImage
	}
	return attachmentKindFile
}

func safeAttachmentName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		name = "attachment"
	}
	var b strings.Builder
	for _, r := range name {
		if r == 0 || r == '/' || r == '\\' || unicode.IsControl(r) || strings.ContainsRune(`<>:"|?*`, r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	name = strings.TrimSpace(b.String())
	name = strings.Trim(name, " .")
	if name == "" {
		return "attachment"
	}
	return truncateAttachmentName(name)
}

// ValidateAttachmentName returns the display name accepted by the attachment store.
func ValidateAttachmentName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "attachment", nil
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("attachment filename %q is unsafe", name)
	}
	for _, r := range name {
		if r == 0 || unicode.IsControl(r) {
			return "", fmt.Errorf("attachment filename %q is unsafe", name)
		}
	}
	if len([]byte(name)) > 255 {
		return "", fmt.Errorf("attachment filename is too long")
	}
	return name, nil
}

func truncateAttachmentName(name string) string {
	if len([]byte(name)) <= maxSafeAttachmentNameBytes {
		return name
	}
	ext := filepath.Ext(name)
	if len([]byte(ext)) > 20 {
		ext = ""
	}
	base := strings.TrimSuffix(name, ext)
	base = strings.TrimRight(truncateUTF8Bytes(base, maxSafeAttachmentNameBytes-len([]byte(ext))), " .")
	if base == "" {
		base = "attachment"
	}
	return base + ext
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		encoded := string(r)
		if b.Len()+len(encoded) > maxBytes {
			break
		}
		b.WriteString(encoded)
	}
	return b.String()
}

func newAttachmentID(sha string) (string, error) {
	var randomBytes [16]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", fmt.Errorf("generate attachment id: %w", err)
	}
	shortSHA := sha
	if len(shortSHA) > 12 {
		shortSHA = shortSHA[:12]
	}
	return "att-" + shortSHA + "-" + hex.EncodeToString(randomBytes[:]), nil
}

func newAttachmentDownloadToken() (string, error) {
	var randomBytes [24]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		return "", fmt.Errorf("generate attachment download token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(randomBytes[:]), nil
}

func validAttachmentID(id string) bool {
	if !strings.HasPrefix(id, "att-") || len(id) < 8 || len(id) > 96 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// Attachment metadata is kept separately for rooms and direct sessions.
type attachmentStore string

func roomAttachmentStore(statePath string) attachmentStore {
	return attachmentStore(filepath.Join(filepath.Dir(statePath), assetsDirName))
}

func sessionAttachmentStore(statePath string) attachmentStore {
	return attachmentStore(filepath.Join(string(roomAttachmentStore(statePath)), "session-responses"))
}

func (store attachmentStore) objectPath(id string) string {
	return filepath.Join(string(store), assetObjectsDirName, id+".json")
}

func (store attachmentStore) blobPath(sha string) string {
	prefix := "00"
	if len(sha) >= 2 {
		prefix = sha[:2]
	}
	return filepath.Join(string(store), assetBlobsDirName, "sha256", prefix, sha)
}

func writeAttachmentObject(path string, object attachmentObject) error {
	data, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return fmt.Errorf("encode attachment object: %w", err)
	}
	data = append(data, '\n')
	return atomicWriteFile(path, data, 0o600)
}

func readAttachmentObject(path string) (attachmentObject, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return attachmentObject{}, errAttachmentNotFound
		}
		return attachmentObject{}, fmt.Errorf("read attachment object: %w", err)
	}
	var object attachmentObject
	if err := json.Unmarshal(data, &object); err != nil {
		return attachmentObject{}, fmt.Errorf("decode attachment object: %w", err)
	}
	if !validAttachmentID(object.ID) || !validSHA256(object.SHA256) || object.SizeBytes < 0 || strings.TrimSpace(object.SafeName) == "" {
		return attachmentObject{}, fmt.Errorf("decode attachment object: invalid metadata")
	}
	if object.SessionBatchID != "" && (!validAttachmentID(object.SessionBatchID) || object.SessionBatchID == object.ID || object.WorkspaceRoot == "") {
		return attachmentObject{}, fmt.Errorf("invalid session attachment batch reference")
	}
	if object.WorkspaceRoot == "" {
		if !validSHA256(object.BlobSHA256) {
			return attachmentObject{}, fmt.Errorf("invalid attachment blob reference")
		}
	} else if object.BlobSHA256 != "" || !filepath.IsAbs(object.WorkspaceRoot) || !object.validSessionWorkspacePath() {
		return attachmentObject{}, fmt.Errorf("invalid attachment workspace reference")
	}
	return object, nil
}

func atomicWriteFile(path string, data []byte, perm os.FileMode, operations ...*diagnostics.Operation) error {
	return atomicWriteReader(path, bytes.NewReader(data), perm, operations...)
}

func atomicWriteReader(path string, source io.Reader, perm os.FileMode, operations ...*diagnostics.Operation) error {
	operation := diagnostics.OperationArg(operations)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := writeAndSyncAttachmentTemp(tmp, source, perm, operation); err != nil {
		return err
	}
	renameStart := time.Now()
	renameErr := os.Rename(tmpName, path)
	operation.Observe("im.rename", time.Since(renameStart), 0, 1)
	if err := renameErr; err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	cleanup = false
	return nil
}

func writeAndSyncAttachmentTemp(tmp *os.File, source io.Reader, perm os.FileMode, operation *diagnostics.Operation) error {
	writeStart := time.Now()
	written, copyErr := io.Copy(tmp, source)
	operation.Observe("im.write", time.Since(writeStart), written, 1)
	if err := copyErr; err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	syncStart := time.Now()
	syncErr := tmp.Sync()
	operation.Observe("im.fsync", time.Since(syncStart), 0, 1)
	if err := syncErr; err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	return nil
}

func prepareWorkspaceAttachmentPath(root, relative string) (string, error) {
	root = strings.TrimSpace(root)
	relative = strings.TrimSpace(relative)
	if root == "" || relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("unsafe attachment path")
	}
	cleanRelative := filepath.Clean(filepath.FromSlash(relative))
	if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe attachment path")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve attachment workspace root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve attachment workspace root: %w", err)
	}
	parts := strings.Split(cleanRelative, string(os.PathSeparator))
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "", fmt.Errorf("unsafe attachment path")
	}
	current := resolvedRoot
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe attachment path")
		}
		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(next, 0o700); err == nil {
				current = next
				continue
			} else if !errors.Is(err, os.ErrExist) {
				return "", fmt.Errorf("create attachment workspace dir: %w", err)
			}
			info, err = os.Lstat(next)
		}
		if err != nil {
			return "", fmt.Errorf("inspect attachment workspace dir: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(next)
			if err != nil {
				return "", fmt.Errorf("resolve attachment workspace dir: %w", err)
			}
			if !pathWithinRoot(resolvedRoot, resolved) {
				return "", fmt.Errorf("unsafe attachment path")
			}
			current = resolved
			continue
		}
		if !info.IsDir() {
			return "", fmt.Errorf("attachment workspace path is not a directory")
		}
		current = next
	}
	return filepath.Join(current, parts[len(parts)-1]), nil
}

func pathWithinRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func cleanupAssetFilesForState(statePath string, rooms []Room) error {
	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return nil
	}
	if err := migrateLegacySessionAttachments(statePath); err != nil {
		return err
	}
	live, liveSHA := liveAttachmentRefs(rooms)
	if err := cleanupLocalAttachmentFiles(statePath, live); err != nil {
		return err
	}
	return roomAttachmentStore(statePath).collect(live, liveSHA)
}

func (store attachmentStore) collect(live, liveSHA map[string]struct{}) error {
	objectsDir := filepath.Join(string(store), assetObjectsDirName)
	entries, err := os.ReadDir(objectsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read attachment objects dir: %w", err)
	}

	canSweepBlobs := true
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		path := filepath.Join(objectsDir, entry.Name())
		if _, ok := live[id]; !ok {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove stale attachment object: %w", err)
			}
			continue
		}
		object, err := readAttachmentObject(path)
		if err != nil {
			canSweepBlobs = false
			continue
		}
		if strings.TrimSpace(object.BlobSHA256) != "" {
			liveSHA[object.BlobSHA256] = struct{}{}
		}
	}
	if !canSweepBlobs {
		return nil
	}
	return cleanupUnreferencedAttachmentBlobs(filepath.Join(string(store), assetBlobsDirName, "sha256"), liveSHA)
}

func liveAttachmentRefs(rooms []Room) (map[string]struct{}, map[string]struct{}) {
	live := make(map[string]struct{})
	liveSHA := make(map[string]struct{})
	for _, room := range rooms {
		recordLiveAttachments(live, liveSHA, room.Messages)
		for _, thread := range room.Threads {
			recordLiveAttachments(live, liveSHA, thread.Context)
		}
	}
	return live, liveSHA
}

func recordLiveAttachments(live, liveSHA map[string]struct{}, messages []Message) {
	for _, message := range messages {
		for _, attachment := range message.Attachments {
			if id := strings.TrimSpace(attachment.ID); id != "" {
				live[id] = struct{}{}
			}
			if sha := strings.TrimSpace(attachment.SHA256); validSHA256(sha) {
				liveSHA[sha] = struct{}{}
			}
		}
	}
}

func cleanupUnreferencedAttachmentBlobs(root string, liveSHA map[string]struct{}) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read attachment blobs dir: %w", err)
	}
	for _, prefixEntry := range entries {
		if !prefixEntry.IsDir() {
			continue
		}
		prefixDir := filepath.Join(root, prefixEntry.Name())
		blobEntries, err := os.ReadDir(prefixDir)
		if err != nil {
			return fmt.Errorf("read attachment blob prefix dir: %w", err)
		}
		for _, blobEntry := range blobEntries {
			if blobEntry.IsDir() {
				continue
			}
			if _, ok := liveSHA[blobEntry.Name()]; ok {
				continue
			}
			if err := os.Remove(filepath.Join(prefixDir, blobEntry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove stale attachment blob: %w", err)
			}
		}
	}
	return nil
}

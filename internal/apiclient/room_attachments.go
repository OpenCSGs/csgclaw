package apiclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"csgclaw/internal/apitypes"
)

func roomAttachmentsPath(roomID string) string {
	return "/api/v1/rooms/" + url.PathEscape(roomID) + "/attachments"
}

func (c *Client) ListRoomAttachments(ctx context.Context, roomID string, opts apitypes.RoomAttachmentListOptions) (apitypes.RoomAttachmentList, error) {
	values := url.Values{"query": {opts.Query}, "message_id": {opts.MessageID}, "from": {strconv.Itoa(opts.From)}}
	if opts.Limit != 0 {
		values.Set("limit", strconv.Itoa(opts.Limit))
	}
	var result apitypes.RoomAttachmentList
	err := c.GetJSON(ctx, roomAttachmentsPath(roomID)+"?"+values.Encode(), &result)
	return result, err
}

// DownloadRoomAttachment saves bytes in the CLI's own environment. Linking the
// verified same-directory temporary file atomically publishes it without replacing
// an existing destination, even if another download wins the race.
func (c *Client) DownloadRoomAttachment(ctx context.Context, roomID, attachmentID, output string) (string, error) {
	target, err := filepath.Abs(output)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(target); err == nil {
		return "", fmt.Errorf("output already exists: %s", target)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+roomAttachmentsPath(roomID)+"/"+url.PathEscape(attachmentID), nil)
	if err != nil {
		return "", err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.callerAgentID != "" {
		req.Header.Set("X-CSGClaw-Caller-Agent", c.callerAgentID)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ExtractAPIError(resp)
	}
	size, err := strconv.ParseInt(resp.Header.Get("X-CSGClaw-File-Size"), 10, 64)
	digest := strings.ToLower(resp.Header.Get("X-CSGClaw-File-SHA256"))
	decoded, hashErr := hex.DecodeString(digest)
	if err != nil || size < 0 || hashErr != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("invalid attachment integrity metadata")
	}
	file, err := os.CreateTemp(filepath.Dir(target), ".csgclaw-download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	// Read exactly the declared bytes, then check for unexpected trailing content.
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, size))
	if err != nil {
		return "", fmt.Errorf("download attachment: %w", err)
	}
	var extra [1]byte
	extraN, extraErr := io.ReadFull(resp.Body, extra[:])
	if n != size || extraN != 0 || extraErr != io.EOF || hex.EncodeToString(hash.Sum(nil)) != digest {
		return "", fmt.Errorf("attachment size or SHA256 verification failed")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Link(file.Name(), target); err != nil {
		return "", fmt.Errorf("publish downloaded file: %w", err)
	}
	return target, nil
}

func (c *Client) DeleteRoomAttachment(ctx context.Context, roomID, attachmentID string) error {
	return c.DoNoContent(ctx, http.MethodDelete, roomAttachmentsPath(roomID)+"/"+url.PathEscape(attachmentID))
}

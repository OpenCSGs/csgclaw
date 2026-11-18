package relay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"csgclaw/internal/apitypes"
)

type File struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Size      int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	Status    string `json:"status,omitempty"`
}

type fileScope struct {
	ResponseID string `json:"response_id,omitempty"`
	AgentID    string `json:"agent_id,omitempty"`
	InstanceID string `json:"instance_id,omitempty"`
}

func (c *Client) uploadFile(ctx context.Context, scope fileScope, key string, file File, source func() (io.ReadCloser, error)) (File, error) {
	var uploaded File
	err := c.Request(ctx, http.MethodPost, "/v1/files", key, struct {
		File
		fileScope
		Transfer string `json:"transfer"`
	}{file, scope, "relay"}, &uploaded)
	if err != nil || uploaded.Status == "available" {
		return uploaded, err
	}
	if !safeID(uploaded.ID) {
		return File{}, fmt.Errorf("invalid uploaded file ID")
	}
	body, err := source()
	if err != nil {
		return File{}, err
	}
	defer body.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.endpoint("/v1/files/"+url.PathEscape(uploaded.ID)+"/content"), body)
	if err != nil {
		return File{}, err
	}
	req.ContentLength = file.Size
	if file.Size == 0 {
		req.Body = http.NoBody
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if err := c.authenticate(req.Header); err != nil {
		return File{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return File{}, fmt.Errorf("Relay file upload interrupted")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return File{}, &HTTPError{Status: resp.StatusCode}
	}
	var confirmed File
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&confirmed); err != nil {
		return File{}, err
	}
	if confirmed.ID != uploaded.ID || confirmed.Status != "available" {
		return File{}, fmt.Errorf("Relay did not confirm upload")
	}
	return confirmed, nil
}

// DownloadFile writes only verified contents to the caller's staging file.
func (c *Client) downloadFile(ctx context.Context, scope fileScope, file File, target *os.File) error {
	if !safeID(file.ID) || file.Size < 0 || file.Size > apitypes.MaxAttachmentFileBytes {
		return fmt.Errorf("invalid Relay file reference")
	}
	resp, err := c.request(ctx, http.MethodPost, "/v1/files/"+url.PathEscape(file.ID)+"/content", "", scope)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(target, hash), io.LimitReader(resp.Body, file.Size+1))
	if err != nil || n != file.Size || hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
		return fmt.Errorf("Relay file integrity check failed")
	}
	_, err = target.Seek(0, io.SeekStart)
	return err
}

func (m *Manager) UploadAttachment(ctx context.Context, agentID, connectionID, messageID string, attachment apitypes.MessageAttachment, open func() (io.ReadCloser, error)) (File, error) {
	client, err := m.invocationClient(ctx, agentID, connectionID)
	if err != nil {
		return File{}, err
	}
	key := fmt.Sprintf("input:%x", sha256.Sum256([]byte(messageID+"\x00"+attachment.ID)))
	file := File{Name: attachment.Name, MediaType: attachment.MediaType, Size: attachment.SizeBytes, SHA256: attachment.SHA256}
	var uploaded File
	err = retryFileTransfer(ctx, func() error {
		var err error
		uploaded, err = client.uploadFile(ctx, fileScope{}, key, file, open)
		return err
	})
	return uploaded, err
}

func (m *Manager) DownloadAttachment(ctx context.Context, agentID, connectionID string, file File, target *os.File) error {
	client, err := m.invocationClient(ctx, agentID, connectionID)
	if err != nil {
		return err
	}
	return client.downloadFile(ctx, fileScope{}, file, target)
}

func retryFileTransfer(ctx context.Context, run func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = run(); err == nil {
			return nil
		}
		var remote *HTTPError
		if errors.As(err, &remote) && remote.Status >= 400 && remote.Status < 500 {
			return err
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * 200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return err
}

package apps

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const maxUploadBytes int64 = 50 << 20

// Upload forwards a runtime-validated workspace file to an installed App's
// HTTP service. The runtime retains responsibility for workspace and turn
// admission; this service owns App authorization and upstream credentials.
func (s *Service) Upload(ctx context.Context, agentID, installationID, uploadURI, filename, contentType string, size int64, body io.Reader) (json.RawMessage, error) {
	if s.isReadOnly(agentID) {
		return nil, fmt.Errorf("Agent is read-only")
	}
	if size < 0 || size > maxUploadBytes {
		return nil, fmt.Errorf("upload size exceeds the supported limit")
	}
	if filename == "" || filepath.Base(filename) != filename || strings.ContainsAny(filename, "/\\\x00\r\n") {
		return nil, fmt.Errorf("invalid upload filename")
	}
	if strings.TrimSpace(uploadURI) == "" {
		return nil, fmt.Errorf("upload URI is required")
	}
	if contentType != "" {
		if _, _, err := mime.ParseMediaType(contentType); err != nil {
			return nil, fmt.Errorf("invalid upload content type")
		}
	}
	s.mu.Lock()
	e, err := s.findLocked(agentID, installationID)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if !e.record.Enabled || e.record.Disconnected || e.connection == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("App is not connected or enabled")
	}
	config := copyJSON(e.record.Config)
	credentials := copyJSON(e.record.Credentials)
	conn := e.connection
	s.mu.Unlock()
	if config.Transport != "http" {
		return nil, fmt.Errorf("file upload requires an HTTP App connection")
	}
	base, err := url.Parse(config.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid App service URL")
	}
	target, err := base.Parse(uploadURI)
	if err != nil || target.Scheme != base.Scheme || !strings.EqualFold(target.Host, base.Host) || target.User != nil || target.Fragment != "" {
		return nil, fmt.Errorf("upload URI must have the same origin as the App service")
	}
	credentials, err = s.resolve(ctx, agentID, config, credentials)
	if err != nil {
		return nil, err
	}
	if config.CredentialSource == "feishu_channel" && sha256.Sum256([]byte(credentials.AppID+"\x00"+credentials.AppSecret)) != conn.credentialHash {
		_ = s.RefreshCredentials(ctx, agentID)
		return nil, fmt.Errorf("App credentials changed; retry after reconnecting")
	}
	client, err := connectorHTTPClient(config, credentials, conn.tokens)
	if err != nil {
		return nil, err
	}
	s.bindPlatformLogin(config, client)
	timeout := time.Duration(config.ToolTimeoutSec) * time.Second
	if timeout <= 0 || timeout > 90*time.Second {
		timeout = 90 * time.Second
	}
	uploadCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stop := context.AfterFunc(conn.ctx, cancel)
	defer stop()
	var limited io.Reader = http.NoBody
	if size > 0 {
		if body == nil {
			return nil, fmt.Errorf("upload body is required")
		}
		limited = io.LimitReader(body, size)
	}
	request, err := http.NewRequestWithContext(uploadCtx, http.MethodPut, target.String(), limited)
	if err != nil {
		return nil, fmt.Errorf("cannot create upload request")
	}
	request.ContentLength = size
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	s.mu.Lock()
	e, err = s.findLocked(agentID, installationID)
	allowed := err == nil && e.connection == conn && e.record.Enabled && !e.record.Disconnected
	s.mu.Unlock()
	if !allowed || s.isReadOnly(agentID) {
		return nil, fmt.Errorf("App upload is no longer authorized")
	}
	response, err := client.Do(request)
	if err != nil {
		var detail *ConnectionError
		if errors.As(err, &detail) {
			s.failConnectionDetail(agentID, installationID, conn, detail)
			return nil, detail
		}
		if errors.Is(err, errAuthentication) {
			s.failConnection(agentID, installationID, conn, "authorization_required", "App authorization is no longer valid; reconnect the App")
		}
		return nil, fmt.Errorf("App upload failed; verify the connection and authorization")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("App upload endpoint returned HTTP %d", response.StatusCode)
	}
	// Never return an upstream response that may echo signed URLs or credentials.
	return json.Marshal(map[string]any{"success": true, "installation_id": installationID, "filename": filename, "size": size, "content_type": contentType})
}

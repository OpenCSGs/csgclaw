package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	clientID    string
	credentials CredentialsSource
	base        *url.URL
	token       string
	http        HTTPDoer
}

func NewClient(endpoint, token string, httpClient HTTPDoer) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("relay URL must be an http(s) origin or base path without credentials, query or fragment")
	}
	if strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("relay token is required (CSGCLAW_RELAY_TOKEN)")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 6 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return &Client{base: u, token: token, http: httpClient}, nil
}

func (c *Client) WithClientID(id string) *Client { c.clientID = id; return c }
func (c *Client) authenticate(headers http.Header) error {
	token := c.token
	if c.credentials != nil {
		cred, err := c.credentials()
		if err != nil {
			return err
		}
		token = cred.Token
	}
	headers.Set("Authorization", "Bearer "+token)
	if c.clientID != "" {
		headers.Set("X-CSG-Client-ID", c.clientID)
	}
	headers.Set("X-CSG-Request-Id", uuid.NewString())
	return nil
}
func (c *Client) endpoint(path string) string { return strings.TrimRight(c.base.String(), "/") + path }

// HTTPError retains machine-readable relay errors without including credentials.
type HTTPError struct {
	Status int
	Detail ProtocolError
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("relay HTTP %d: %s (%s)", e.Status, e.Detail.Code, e.Detail.ResponseID)
}

func (c *Client) Request(ctx context.Context, method, path, key string, body, out any) error {
	resp, err := c.request(ctx, method, path, key, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

func (c *Client) request(ctx context.Context, method, path, key string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		if len(data) > 1<<20 {
			return nil, fmt.Errorf("relay request exceeds 1 MiB")
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path), reader)
	if err != nil {
		return nil, err
	}
	if err := c.authenticate(req.Header); err != nil {
		return nil, err
	}
	if key == "" {
		key = uuid.NewString()
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		if len(key) > 128 {
			return nil, fmt.Errorf("idempotency key exceeds 128 bytes")
		}
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("relay transport failed")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		var envelope struct {
			Error ProtocolError `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&envelope)
		return nil, &HTTPError{Status: resp.StatusCode, Detail: envelope.Error}
	}
	return resp, nil
}

func (c *Client) Publish(ctx context.Context, key string, req PublishRequest) (Publication, error) {
	var p Publication
	err := c.Request(ctx, http.MethodPost, "/v1/agents", key, req, &p)
	return p, err
}
func (c *Client) Call(ctx context.Context, key string, req CallRequest) (Response, error) {
	var r Response
	req.Stream = false
	err := c.Request(ctx, http.MethodPost, "/v1/responses", key, req, &r)
	return r, err
}
func (c *Client) Stream(ctx context.Context, key string, req CallRequest, out io.Writer) error {
	req.Stream = true
	resp, err := c.request(ctx, http.MethodPost, "/v1/responses", key, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return fmt.Errorf("relay did not return an SSE stream")
	}
	_, err = io.Copy(out, resp.Body)
	return err
}

func (c *Client) connect(ctx context.Context, agentID string, instance ...string) (*websocket.Conn, error) {
	u := *c.base
	u.Path += "/v1/agents/" + url.PathEscape(agentID) + "/connect"
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	d := websocket.Dialer{Subprotocols: []string{Subprotocol}, HandshakeTimeout: 10 * time.Second, Proxy: http.ProxyFromEnvironment}
	headers := http.Header{}
	if err := c.authenticate(headers); err != nil {
		return nil, err
	}
	headers.Set("Idempotency-Key", uuid.NewString())
	if len(instance) > 0 {
		headers.Set("X-CSGRelay-Instance", instance[0])
	}
	conn, resp, err := d.DialContext(ctx, u.String(), headers)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("relay executor connection failed")
	}
	if conn.Subprotocol() != Subprotocol {
		conn.Close()
		return nil, fmt.Errorf("relay did not negotiate %s", Subprotocol)
	}
	conn.SetReadLimit(2 << 20)
	return conn, nil
}

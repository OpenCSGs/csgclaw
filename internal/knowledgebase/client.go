// Package knowledgebase integrates AgenticHub LLM-Wiki knowledge bases with
// CSGClaw's MCP catalog without persisting user credentials.
package knowledgebase

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const TypeLLMWiki = "llmwiki"

type ListOptions struct {
	Page   int
	Per    int
	Search string
}

type ListResult struct {
	Items    []KnowledgeBase
	RawItems []json.RawMessage
	Total    int
}

type KnowledgeBase struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	ContentID   string   `json:"content_id"`
	Type        string   `json:"type"`
	Public      bool     `json:"public"`
	Editable    bool     `json:"editable"`
	IsPinned    bool     `json:"is_pinned"`
	Metadata    Metadata `json:"metadata"`
}

type Metadata struct {
	BaseURL       string         `json:"base_url"`
	MCPEndpoint   string         `json:"mcp_endpoint_url"`
	ResourceState *ResourceState `json:"resource_state"`
}

type ResourceState struct {
	Readiness     string          `json:"readiness"`
	MCPStatus     string          `json:"mcp_status"`
	MCPEndpoint   string          `json:"mcp_endpoint_url"`
	LastError     json.RawMessage `json:"last_error"`
	RuntimeStatus string          `json:"runtime_status"`
}

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("AgenticHub request failed with status %d", e.StatusCode)
}

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func (c Client) List(ctx context.Context, options ListOptions) (ListResult, error) {
	page := options.Page
	if page < 1 {
		page = 1
	}
	per := options.Per
	if per < 1 {
		per = 20
	}
	endpoint, err := c.endpoint("/api/v1/agent/knowledge-bases")
	if err != nil {
		return ListResult{}, err
	}
	query := endpoint.Query()
	query.Set("type", TypeLLMWiki)
	query.Set("page", strconv.Itoa(page))
	query.Set("per", strconv.Itoa(per))
	if search := strings.TrimSpace(options.Search); search != "" {
		query.Set("search", search)
	}
	endpoint.RawQuery = query.Encode()

	var response struct {
		Data  []json.RawMessage `json:"data"`
		Total int               `json:"total"`
	}
	if _, err := c.getJSON(ctx, endpoint.String(), &response); err != nil {
		return ListResult{}, err
	}
	items := make([]KnowledgeBase, 0, len(response.Data))
	rawItems := make([]json.RawMessage, 0, len(response.Data))
	for _, rawItem := range response.Data {
		var item KnowledgeBase
		if err := json.Unmarshal(rawItem, &item); err != nil {
			return ListResult{}, fmt.Errorf("decode AgenticHub knowledge base item: %w", err)
		}
		items = append(items, normalizeKnowledgeBase(item))
		rawItems = append(rawItems, rawItem)
	}
	return ListResult{Items: items, RawItems: rawItems, Total: response.Total}, nil
}

func (c Client) Get(ctx context.Context, id int64) (KnowledgeBase, error) {
	if id < 1 {
		return KnowledgeBase{}, fmt.Errorf("knowledge base id is required")
	}
	endpoint, err := c.endpoint("/api/v1/agent/knowledge-bases/" + strconv.FormatInt(id, 10))
	if err != nil {
		return KnowledgeBase{}, err
	}
	var response struct {
		Data KnowledgeBase `json:"data"`
	}
	if _, err := c.getJSON(ctx, endpoint.String(), &response); err != nil {
		return KnowledgeBase{}, err
	}
	item := normalizeKnowledgeBase(response.Data)
	if !strings.EqualFold(item.Type, TypeLLMWiki) {
		return KnowledgeBase{}, fmt.Errorf("knowledge base %d is not an LLM-Wiki resource", id)
	}
	return item, nil
}

func (c Client) endpoint(path string) (*url.URL, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("CSGHub base URL is required")
	}
	endpoint, err := url.Parse(baseURL + path)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("invalid CSGHub base URL")
	}
	return endpoint, nil
}

func (c Client) getJSON(ctx context.Context, endpoint string, target any) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(c.Token)
	if token == "" {
		return nil, fmt.Errorf("OpenCSG sign-in is required")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request AgenticHub knowledge bases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(readErrorMessage(resp.Body))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, &HTTPError{StatusCode: resp.StatusCode, Message: message}
	}
	var rawResponse json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return nil, fmt.Errorf("decode AgenticHub knowledge base response: %w", err)
	}
	if err := json.Unmarshal(rawResponse, target); err != nil {
		return nil, fmt.Errorf("decode AgenticHub knowledge base response: %w", err)
	}
	return rawResponse, nil
}

func normalizeKnowledgeBase(item KnowledgeBase) KnowledgeBase {
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.ContentID = strings.TrimSpace(item.ContentID)
	item.Type = strings.ToLower(strings.TrimSpace(item.Type))
	item.Metadata.BaseURL = strings.TrimSpace(item.Metadata.BaseURL)
	item.Metadata.MCPEndpoint = strings.TrimSpace(item.Metadata.MCPEndpoint)
	if item.Metadata.ResourceState != nil {
		item.Metadata.ResourceState.Readiness = strings.ToLower(strings.TrimSpace(item.Metadata.ResourceState.Readiness))
		item.Metadata.ResourceState.MCPStatus = strings.ToLower(strings.TrimSpace(item.Metadata.ResourceState.MCPStatus))
		item.Metadata.ResourceState.MCPEndpoint = strings.TrimSpace(item.Metadata.ResourceState.MCPEndpoint)
		item.Metadata.ResourceState.RuntimeStatus = strings.ToLower(strings.TrimSpace(item.Metadata.ResourceState.RuntimeStatus))
	}
	return item
}

func readErrorMessage(body io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(body, 32<<10))
	if err != nil || len(data) == 0 {
		return ""
	}
	var response struct {
		Message string `json:"message"`
		Msg     string `json:"msg"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(data, &response) == nil {
		for _, value := range []string{response.Message, response.Msg, response.Error} {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return strings.TrimSpace(string(data))
}

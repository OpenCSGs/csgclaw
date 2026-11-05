// Package relay implements the CSGRelay public HTTP and executor protocols.
package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const Subprotocol = "csgrelay.v1"

type Agent struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	OwnerID       string     `json:"owner_id"`
	PublisherID   string     `json:"publisher_id"`
	Visibility    string     `json:"visibility"`
	Executor      string     `json:"executor"`
	SourceAgentID string     `json:"source_agent_id"`
	Status        string     `json:"status"`
	StatusReason  string     `json:"status_reason,omitempty"`
	Busy          bool       `json:"busy"`
	LastSeenAt    *time.Time `json:"last_seen_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type PublishRequest struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Visibility    string `json:"visibility,omitempty"`
	Executor      string `json:"executor,omitempty"`
	SourceAgentID string `json:"source_agent_id"`
}

type Publication struct {
	Agent       Agent  `json:"agent"`
	ConnectPath string `json:"connect_path"`
}

type AgentList struct {
	Object     string  `json:"object"`
	Data       []Agent `json:"data"`
	NextCursor *string `json:"next_cursor"`
}

type CallRequest struct {
	Model              string          `json:"model"`
	Input              json.RawMessage `json:"input"`
	Stream             bool            `json:"stream,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
}

type ProtocolError struct {
	Code       string  `json:"code"`
	Message    string  `json:"message"`
	Type       string  `json:"type,omitempty"`
	Param      *string `json:"param"`
	ResponseID string  `json:"response_id,omitempty"`
}

type Response struct {
	ID                 string         `json:"id"`
	Object             string         `json:"object"`
	CreatedAt          int64          `json:"created_at"`
	Status             string         `json:"status"`
	Model              string         `json:"model"`
	PreviousResponseID *string        `json:"previous_response_id"`
	Output             []Output       `json:"output"`
	Error              *ProtocolError `json:"error"`
	IncompleteDetails  any            `json:"incomplete_details"`
}

type Output struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"`
	Role    string    `json:"role"`
	Status  string    `json:"status"`
	Content []Content `json:"content"`
}

type Content struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
}

func (r Response) Text() string {
	var text strings.Builder
	for _, output := range r.Output {
		for _, part := range output.Content {
			if part.Type == "output_text" {
				text.WriteString(part.Text)
			}
		}
	}
	return text.String()
}

type Frame struct {
	parentValidated          bool
	Type                     string          `json:"type"`
	ProtocolVersion          int             `json:"protocol_version,omitempty"`
	InstanceID               string          `json:"instance_id,omitempty"`
	Ready                    bool            `json:"ready"`
	Busy                     bool            `json:"busy"`
	Reason                   string          `json:"reason,omitempty"`
	ConnectionID             string          `json:"connection_id,omitempty"`
	HeartbeatIntervalSeconds int             `json:"heartbeat_interval_seconds,omitempty"`
	HeartbeatTimeoutSeconds  int             `json:"heartbeat_timeout_seconds,omitempty"`
	ResponseID               string          `json:"response_id,omitempty"`
	SessionID                string          `json:"session_id,omitempty"`
	PreviousResponseID       string          `json:"previous_response_id,omitempty"`
	Input                    json.RawMessage `json:"input,omitempty"`
	Deadline                 time.Time       `json:"deadline,omitzero"`
	Sequence                 uint64          `json:"sequence,omitempty"`
	Text                     string          `json:"text,omitempty"`
	OutputText               string          `json:"output_text,omitempty"`
	Error                    *ProtocolError  `json:"error,omitempty"`
}

// InputText accepts only the text-only subset in the public relay contract.
func InputText(raw json.RawMessage) (string, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("input must not be empty")
		}
		return text, nil
	}
	var messages []struct {
		Type    string          `json:"type,omitempty"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := strictJSON(raw, &messages); err != nil || len(messages) == 0 {
		return "", fmt.Errorf("input must be text or user text messages")
	}
	var parts []string
	for _, message := range messages {
		if message.Role != "user" || (message.Type != "" && message.Type != "message") {
			return "", fmt.Errorf("only user text messages are supported")
		}
		if json.Unmarshal(message.Content, &text) == nil {
			parts = append(parts, text)
			continue
		}
		var content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := strictJSON(message.Content, &content); err != nil || len(content) == 0 {
			return "", fmt.Errorf("only input_text content is supported")
		}
		for _, part := range content {
			if part.Type != "input_text" {
				return "", fmt.Errorf("only input_text content is supported")
			}
			parts = append(parts, part.Text)
		}
	}
	text = strings.Join(parts, "\n")
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("input must not be empty")
	}
	return text, nil
}

func strictJSON(raw []byte, out any) error {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("expected one JSON value")
	}
	return nil
}

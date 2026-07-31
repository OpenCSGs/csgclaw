package desktop

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	appversion "csgclaw/internal/version"
)

const (
	ProtocolVersion = 1

	MessageTypeBootstrap = "csgclaw.desktop.bootstrap"
	MessageTypeReady     = "csgclaw.desktop.ready"
	MessageTypeShutdown  = "csgclaw.desktop.shutdown"
)

type BootstrapMessage struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocol_version"`
	InstanceID      string `json:"instance_id"`
	SessionToken    string `json:"session_token"`
}

func (m BootstrapMessage) Validate() error {
	if m.Type != MessageTypeBootstrap {
		return fmt.Errorf("unexpected desktop bootstrap message type")
	}
	if m.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("unsupported desktop protocol version %d", m.ProtocolVersion)
	}
	if len(strings.TrimSpace(m.InstanceID)) < 16 {
		return fmt.Errorf("desktop instance id is invalid")
	}
	if len(strings.TrimSpace(m.SessionToken)) < 43 {
		return fmt.Errorf("desktop session token must contain at least 256 bits")
	}
	return nil
}

type ReadyMessage struct {
	Type            string `json:"type"`
	ProtocolVersion int    `json:"protocol_version"`
	InstanceID      string `json:"instance_id"`
	PID             int    `json:"pid"`
	BaseURL         string `json:"base_url"`
	Version         string `json:"version"`
	Distribution    string `json:"distribution"`
}

func NewReadyMessage(instanceID, baseURL string) (ReadyMessage, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" || parsed.User != nil {
		return ReadyMessage{}, fmt.Errorf("desktop base URL is invalid")
	}
	if parsed.Port() == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ReadyMessage{}, fmt.Errorf("desktop base URL must be a loopback origin")
	}
	return ReadyMessage{
		Type:            MessageTypeReady,
		ProtocolVersion: ProtocolVersion,
		InstanceID:      instanceID,
		PID:             os.Getpid(),
		BaseURL:         parsed.String(),
		Version:         appversion.Current(),
		Distribution:    "electron",
	}, nil
}

type ControlMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason,omitempty"`
}

package dockercli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"csgclaw/internal/sandbox"
)

type inspectContainer struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Created string `json:"Created"`
	State   struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
	} `json:"State"`
}

func parseInspect(data []byte) (sandbox.Info, error) {
	var containers []inspectContainer
	if err := json.Unmarshal(data, &containers); err != nil {
		return sandbox.Info{}, fmt.Errorf("parse docker inspect json: %w", err)
	}
	if len(containers) == 0 {
		return sandbox.Info{}, sandbox.ErrNotFound
	}
	c := containers[0]
	createdAt, err := parseCreatedAt(c.Created)
	if err != nil {
		return sandbox.Info{}, err
	}
	name := strings.TrimPrefix(c.Name, "/")
	status := c.State.Status
	if status == "" && c.State.Running {
		status = "running"
	}
	return sandbox.Info{
		ID:        c.ID,
		Name:      name,
		State:     mapState(status),
		CreatedAt: createdAt,
	}, nil
}

func parseCreatedAt(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	createdAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		createdAt, err = time.Parse(time.RFC3339, value)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("parse docker created time %q: %w", value, err)
	}
	return createdAt, nil
}

func mapState(status string) sandbox.State {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "created":
		return sandbox.StateCreated
	case "running":
		return sandbox.StateRunning
	case "paused", "restarting":
		return sandbox.StateUnknown
	case "removing":
		return sandbox.StateUnknown
	case "exited", "dead":
		return sandbox.StateExited
	default:
		return sandbox.StateUnknown
	}
}

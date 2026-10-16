package larkcli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

type ConfigIdentity struct {
	AppID  string
	Name   string
	Brand  string
	Secret json.RawMessage
}

// SelectConfig preserves the selected profile and its credential references.
// The connector owns exactly one application; auxiliary profiles remain in the
// migration backup owned by CSGClaw.
func SelectConfig(data []byte) ([]byte, ConfigIdentity, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, ConfigIdentity{}, err
	}
	var apps []map[string]json.RawMessage
	if err := json.Unmarshal(root["apps"], &apps); err != nil || len(apps) == 0 {
		return nil, ConfigIdentity{}, errors.New("CLI configuration has no application")
	}
	var current string
	_ = json.Unmarshal(root["currentApp"], &current)
	selected := -1
	for i, app := range apps {
		var id, name string
		_ = json.Unmarshal(app["appId"], &id)
		_ = json.Unmarshal(app["name"], &name)
		if (current == "" && i == 0) || current == id || current == name {
			selected = i
			break
		}
	}
	if selected < 0 {
		return nil, ConfigIdentity{}, errors.New("CLI selected application is unavailable")
	}
	app := apps[selected]
	identity := ConfigIdentity{Secret: app["appSecret"]}
	_ = json.Unmarshal(app["appId"], &identity.AppID)
	_ = json.Unmarshal(app["name"], &identity.Name)
	_ = json.Unmarshal(app["brand"], &identity.Brand)
	if identity.AppID == "" {
		return nil, ConfigIdentity{}, errors.New("CLI application ID is missing")
	}
	root["apps"], _ = json.Marshal([]map[string]json.RawMessage{app})
	delete(root, "currentApp")
	delete(root, "previousApp")
	data, err := json.Marshal(root)
	return data, identity, err
}

// ImportConfig uses the official setter to validate and save the preserved
// identity policy. Existing keychain entries and refreshed user tokens stay in
// their original credential store.
func (c Client) ImportConfig(ctx context.Context, data []byte, secret string) (Status, error) {
	selected, identity, err := SelectConfig(data)
	if err != nil {
		return Status{}, err
	}
	if err = AtomicWrite(c.ConfigPath(), selected); err != nil {
		return Status{}, err
	}
	if secret != "" {
		// A named update retains users and policies for the same application.
		var root map[string]json.RawMessage
		_ = json.Unmarshal(selected, &root)
		var apps []map[string]json.RawMessage
		_ = json.Unmarshal(root["apps"], &apps)
		name := identity.Name
		if name == "" {
			name = "csgclaw"
			apps[0]["name"], _ = json.Marshal(name)
			root["apps"], _ = json.Marshal(apps)
			selected, _ = json.Marshal(root)
			if err = AtomicWrite(c.ConfigPath(), selected); err != nil {
				return Status{}, err
			}
		}
		_, err = c.Run(ctx, strings.NewReader(secret+"\n"), "config", "init", "--app-id", identity.AppID, "--app-secret-stdin", "--brand", identity.Brand, "--name", name)
		if err != nil {
			return Status{}, err
		}
	}
	var root struct {
		StrictMode string `json:"strictMode"`
	}
	if err = json.Unmarshal(selected, &root); err != nil {
		return Status{}, err
	}
	mode := root.StrictMode
	if mode == "" {
		mode = "off"
	}
	if _, err = c.Run(ctx, nil, "config", "strict-mode", mode, "--global"); err != nil {
		return Status{}, err
	}
	status, err := c.Status(ctx, false)
	if err != nil {
		return Status{}, err
	}
	if status.AppID != identity.AppID {
		return Status{}, errors.New("migrated application identity does not match")
	}
	return status, nil
}

func ReadConfigIdentity(filename string) (ConfigIdentity, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return ConfigIdentity{}, err
	}
	_, identity, err := SelectConfig(data)
	return identity, err
}

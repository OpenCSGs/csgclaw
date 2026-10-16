package connectorcli

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// JSONMatch uses JSON pointers. Every field in one alternative must match.
type JSONMatch []map[string]any

type Check struct {
	Args            []string  `json:"args"`
	MatchJSON       JSONMatch `json:"match_json"`
	UnavailableJSON JSONMatch `json:"unavailable_json,omitempty"`
}
type URLRewrite struct {
	Path      string                     `json:"path,omitempty"`
	Query     map[string]string          `json:"query,omitempty"`
	GzipQuery map[string]json.RawMessage `json:"gzip_query,omitempty"`
}
type DeviceFlow struct {
	CodeField       string    `json:"code_field"`
	URLField        string    `json:"url_field"`
	ExpiresField    string    `json:"expires_in_field"`
	PollArgs        []string  `json:"poll_args"`
	AcceptErrorJSON JSONMatch `json:"accept_error_json,omitempty"`
}
type Command struct {
	Args            []string    `json:"args"`
	Stage           string      `json:"stage,omitempty"`
	When            *Check      `json:"when,omitempty"`
	Unless          *Check      `json:"unless,omitempty"`
	MatchJSON       JSONMatch   `json:"match_json,omitempty"`
	AcceptErrorJSON JSONMatch   `json:"accept_error_json,omitempty"`
	URLRewrite      *URLRewrite `json:"url_rewrite,omitempty"`
	DeviceFlow      *DeviceFlow `json:"device_flow,omitempty"`
}

// Commands also accepts a single argument array for simple connectors.
type Commands []Command

func (c *Commands) UnmarshalJSON(data []byte) error {
	var args []string
	if err := json.Unmarshal(data, &args); err == nil {
		*c = nil
		if len(args) > 0 {
			*c = Commands{{Args: args}}
		}
		return nil
	}
	type commands Commands
	return json.Unmarshal(data, (*commands)(c))
}
func pointerParts(pointer string) ([]string, error) {
	if !strings.HasPrefix(pointer, "/") {
		return nil, errors.New("JSON field must use a nonempty JSON pointer")
	}
	parts := strings.Split(pointer[1:], "/")
	for i, p := range parts {
		if strings.Contains(regexp.MustCompile(`~[01]`).ReplaceAllString(p, ""), "~") {
			return nil, errors.New("invalid JSON pointer escape")
		}
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")
	}
	return parts, nil
}
func JSONValue(value any, pointer string) (any, bool) {
	parts, err := pointerParts(pointer)
	if err != nil {
		return nil, false
	}
	for _, part := range parts {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

// DecodeJSON tolerates command notices before a JSON response and JSON events.
func DecodeJSON(data []byte) (map[string]any, error) {
	for i, b := range data {
		if b != '{' {
			continue
		}
		var object map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(data[i:]), &object); err == nil {
			return object, nil
		}
	}
	return nil, errors.New("invalid CLI JSON response")
}
func (m JSONMatch) Validate() error {
	for _, alternative := range m {
		if len(alternative) == 0 {
			return errors.New("empty JSON match")
		}
		for pointer, value := range alternative {
			if _, err := pointerParts(pointer); err != nil {
				return err
			}
			switch value.(type) {
			case nil, bool, string, float64, int:
			default:
				return errors.New("JSON match values must be scalar")
			}
		}
	}
	return nil
}
func (m JSONMatch) Matches(object map[string]any) bool {
	for _, alternative := range m {
		matched := true
		for pointer, expected := range alternative {
			actual, ok := JSONValue(object, pointer)
			if !ok || !reflect.DeepEqual(actual, expected) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
func matchesOutput(data []byte, match JSONMatch) bool {
	object, err := DecodeJSON(data)
	return err == nil && match.Matches(object)
}
func (c Commands) Validate() error {
	for _, command := range c {
		if len(command.Args) == 0 {
			return errors.New("CLI command requires arguments")
		}
		if err := command.MatchJSON.Validate(); err != nil {
			return err
		}
		if err := command.AcceptErrorJSON.Validate(); err != nil {
			return err
		}
		for _, check := range []*Check{command.When, command.Unless} {
			if check != nil {
				if len(check.Args) == 0 || len(check.MatchJSON) == 0 {
					return errors.New("CLI check requires arguments and JSON match")
				}
				if err := check.MatchJSON.Validate(); err != nil {
					return err
				}
				if err := check.UnavailableJSON.Validate(); err != nil {
					return err
				}
			}
		}
		if flow := command.DeviceFlow; flow != nil {
			for _, pointer := range []string{flow.CodeField, flow.URLField, flow.ExpiresField} {
				if _, err := pointerParts(pointer); err != nil {
					return err
				}
			}
			if len(flow.PollArgs) == 0 || !strings.Contains(strings.Join(flow.PollArgs, " "), "${DEVICE_CODE}") {
				return errors.New("device flow requires a polling command with DEVICE_CODE")
			}
			if err := flow.AcceptErrorJSON.Validate(); err != nil {
				return err
			}
		}
		if rewrite := command.URLRewrite; rewrite != nil {
			if rewrite.Path != "" && (!strings.HasPrefix(rewrite.Path, "/") || strings.ContainsAny(rewrite.Path, "?#")) {
				return errors.New("invalid authorization URL path")
			}
			for _, data := range rewrite.GzipQuery {
				if !json.Valid(data) {
					return errors.New("invalid authorization URL JSON")
				}
			}
		}
	}
	return nil
}
func (r URLRewrite) Apply(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if r.Path != "" {
		u.Path = r.Path
		u.RawPath = ""
	}
	query := u.Query()
	for key, value := range r.Query {
		query.Set(key, value)
	}
	for key, data := range r.GzipQuery {
		var compact bytes.Buffer
		if err := json.Compact(&compact, data); err != nil {
			return "", err
		}
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(compact.Bytes()); err != nil {
			return "", err
		}
		if err := writer.Close(); err != nil {
			return "", err
		}
		query.Set(key, base64.RawURLEncoding.EncodeToString(compressed.Bytes()))
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

type DeviceAuthorization struct {
	Command   string    `json:"command"`
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Workflow struct {
	Client          Client
	RequestedScopes string
	Device          *DeviceAuthorization
	SaveDevice      func(*DeviceAuthorization) error
	Stage           func(string) error
	// StageCompleted runs after a named stage succeeds, including an already satisfied Unless check.
	StageCompleted func(context.Context) error
}

func (w *Workflow) args(args []string) []string {
	result := make([]string, len(args))
	for i, arg := range args {
		result[i] = strings.ReplaceAll(arg, "${REQUESTED_SCOPES}", w.RequestedScopes)
	}
	return result
}
func (w *Workflow) Check(ctx context.Context, check *Check) (bool, error) {
	client := w.Client
	client.URL = nil
	data, err := client.Run(ctx, nil, w.args(check.Args)...)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if matchesOutput(data, check.UnavailableJSON) {
			return false, nil
		}
		return false, err
	}
	object, err := DecodeJSON(data)
	if err != nil {
		return false, err
	}
	return check.MatchJSON.Matches(object), nil
}
func (w *Workflow) saveDevice(device *DeviceAuthorization) error {
	if w.SaveDevice != nil {
		if err := w.SaveDevice(device); err != nil {
			return err
		}
	}
	w.Device = device
	return nil
}
func (w *Workflow) Run(ctx context.Context, commands Commands) error {
	for _, command := range commands {
		if err := ctx.Err(); err != nil {
			return err
		}
		if command.When != nil {
			matched, err := w.Check(ctx, command.When)
			if err != nil {
				return err
			}
			if !matched {
				continue
			}
		}
		if command.Unless != nil {
			matched, err := w.Check(ctx, command.Unless)
			if err != nil {
				return err
			}
			if matched {
				if err := w.completeStage(ctx, command); err != nil {
					return err
				}
				continue
			}
		}
		if w.Stage != nil && command.Stage != "" {
			if err := w.Stage(command.Stage); err != nil {
				return err
			}
		}
		if err := w.run(ctx, command); err != nil {
			return err
		}
		if err := w.completeStage(ctx, command); err != nil {
			return err
		}
	}
	return ctx.Err()
}
func (w *Workflow) completeStage(ctx context.Context, command Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if command.Stage != "" && w.StageCompleted != nil {
		return w.StageCompleted(ctx)
	}
	return nil
}
func (w *Workflow) run(ctx context.Context, command Command) error {
	client := w.Client
	var urlErr error
	notify := client.URL
	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	client.URL = func(raw string) {
		if !AllowedAuthorizationURL(raw, client.AuthorizationDomains) {
			urlErr = errors.New("invalid authorization URL")
			cancel()
			return
		}
		if command.URLRewrite != nil {
			raw, urlErr = command.URLRewrite.Apply(raw)
			if urlErr != nil {
				cancel()
				return
			}
		}
		if notify != nil {
			notify(raw)
		}
	}
	args := w.args(command.Args)
	if command.DeviceFlow != nil {
		return w.deviceFlow(commandCtx, client, command, args)
	}
	data, err := client.Run(commandCtx, nil, args...)
	if urlErr != nil {
		return urlErr
	}
	if commandCtx.Err() != nil {
		return commandCtx.Err()
	}
	if err != nil && !matchesOutput(data, command.AcceptErrorJSON) {
		return err
	}
	if len(command.MatchJSON) > 0 && !matchesOutput(data, command.MatchJSON) {
		return errors.New("CLI command did not satisfy its JSON condition")
	}
	return nil
}
func (w *Workflow) deviceFlow(ctx context.Context, client Client, command Command, args []string) error {
	flow := command.DeviceFlow
	encoded, _ := json.Marshal(struct {
		Command Command
		Scopes  string
	}{command, w.RequestedScopes})
	digest := sha256.Sum256(encoded)
	key := hex.EncodeToString(digest[:])
	device := w.Device
	if device == nil || device.Command != key || !time.Now().Before(device.ExpiresAt) {
		data, err := client.Run(ctx, nil, args...)
		if err != nil {
			return err
		}
		object, err := DecodeJSON(data)
		if err != nil {
			return err
		}
		rawCode, _ := JSONValue(object, flow.CodeField)
		code, _ := rawCode.(string)
		rawURL, _ := JSONValue(object, flow.URLField)
		address, _ := rawURL.(string)
		rawExpiry, _ := JSONValue(object, flow.ExpiresField)
		seconds, _ := rawExpiry.(float64)
		if code == "" || !AllowedAuthorizationURL(address, client.AuthorizationDomains) || seconds <= 0 || seconds > 86400 {
			return errors.New("invalid device authorization response")
		}
		device = &DeviceAuthorization{Command: key, Code: code, URL: address, ExpiresAt: time.Now().Add(time.Duration(seconds) * time.Second)}
		if err := w.saveDevice(device); err != nil {
			return err
		}
	}
	if client.URL != nil {
		client.URL(device.URL)
	}
	poll := w.args(flow.PollArgs)
	for i, arg := range poll {
		poll[i] = strings.ReplaceAll(arg, "${DEVICE_CODE}", device.Code)
	}
	pollCtx, cancel := context.WithDeadline(ctx, device.ExpiresAt)
	defer cancel()
	data, err := client.Run(pollCtx, nil, poll...)
	if pollCtx.Err() != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &OperationError{Code: "authorization_expired"}
	}
	if err != nil && !matchesOutput(data, flow.AcceptErrorJSON) {
		return err
	}
	return w.saveDevice(nil)
}

// Status only exposes fields explicitly selected by the package.
func (s Spec) ReadStatus(data []byte) (bool, json.RawMessage, error) {
	var object map[string]any
	if len(s.StatusMatchJSON) > 0 || len(s.StatusFields) > 0 {
		var err error
		object, err = DecodeJSON(data)
		if err != nil {
			return false, nil, err
		}
	}
	ready := false
	if len(s.StatusMatchJSON) > 0 {
		ready = s.StatusMatchJSON.Matches(object)
	} else {
		match, err := regexp.Compile(s.StatusMatch)
		if err != nil {
			return false, nil, err
		}
		ready = match.Match(data)
	}
	selected := map[string]any{}
	for _, pointer := range s.StatusFields {
		value, ok := JSONValue(object, pointer)
		if !ok {
			continue
		}
		parts, err := pointerParts(pointer)
		if err != nil {
			return false, nil, err
		}
		target := selected
		for _, part := range parts[:len(parts)-1] {
			if _, exists := target[part]; !exists {
				target[part] = map[string]any{}
			}
			var ok bool
			target, ok = target[part].(map[string]any)
			if !ok {
				return false, nil, fmt.Errorf("overlapping status fields")
			}
		}
		target[parts[len(parts)-1]] = value
	}
	if len(selected) == 0 {
		return ready, nil, nil
	}
	status, err := json.Marshal(selected)
	return ready, status, err
}

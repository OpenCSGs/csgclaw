package larkcli

import (
	"bytes"
	"context"
	"csgclaw/internal/connectorcli"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"

	projection "csgclaw/internal/runtimeextension/larkcli"
)

type Identity struct {
	Status      string `json:"status"`
	Available   bool   `json:"available"`
	Verified    *bool  `json:"verified,omitempty"`
	OpenID      string `json:"openId,omitempty"`
	AppName     string `json:"appName,omitempty"`
	UserName    string `json:"userName,omitempty"`
	Scope       string `json:"scope,omitempty"`
	TokenStatus string `json:"tokenStatus,omitempty"`
}
type Status struct {
	AppID      string `json:"appId"`
	DefaultAs  string `json:"defaultAs"`
	Identities struct {
		Bot  Identity `json:"bot"`
		User Identity `json:"user"`
	} `json:"identities"`
}
type ProcessRef = connectorcli.ProcessRef

var RecoverProcess = connectorcli.RecoverProcess
var AtomicWrite = connectorcli.AtomicWrite

type Client struct {
	Env                  map[string]string
	AuthorizationDomains []string
	Runner               func(context.Context, Client, io.Reader, ...string) ([]byte, error)
	Executable           string
	ConfigDir            string
	OperationID          string
	Process              func(ProcessRef) error
	URL                  func(string)
}

func (c Client) command() connectorcli.Client {
	env := c.Env
	if env == nil {
		env = map[string]string{"LARKSUITE_CLI_CONFIG_DIR": c.ConfigDir}
	}
	domains := c.AuthorizationDomains
	if domains == nil {
		domains = []string{"feishu.cn", "larksuite.com"}
	}

	return connectorcli.Client{Executable: c.Executable, ConfigDir: c.ConfigDir, OperationID: c.OperationID, Process: c.Process, URL: c.URL,
		Env: env, ExcludeEnv: projection.IsConfigurationEnv, AuthorizationDomains: domains}
}
func (c Client) Run(ctx context.Context, input io.Reader, args ...string) ([]byte, error) {
	if c.Runner != nil {
		return c.Runner(ctx, c, input, args...)
	}
	return c.command().Run(ctx, input, args...)
}
func (c Client) Status(ctx context.Context, verify bool) (Status, error) {
	var result Status
	args := []string{"auth", "status", "--json"}
	if verify {
		args = append(args, "--verify")
	}
	data, err := c.Run(ctx, nil, args...)
	if err != nil {
		return result, err
	}
	if err := decodeObject(data, &result); err != nil {
		return result, err
	}
	if result.AppID == "" {
		return result, errors.New("CLI status has no application identity")
	}
	return result, nil
}
func decodeObject(data []byte, target any) error {
	// 官方命令可能先输出提示，再输出 JSON。
	for index, b := range data {
		if b == '{' && json.Unmarshal(bytes.TrimSpace(data[index:]), target) == nil {
			return nil
		}
	}
	return errors.New("invalid CLI JSON response")
}
func (c Client) ConfigPath() string { return filepath.Join(c.ConfigDir, "config.json") }
func (c Client) WithRunner(runner connectorcli.Runner) Client {
	if runner != nil {
		c.Runner = func(ctx context.Context, current Client, input io.Reader, args ...string) ([]byte, error) {
			return runner(ctx, current.command(), input, args...)
		}
	}
	return c
}

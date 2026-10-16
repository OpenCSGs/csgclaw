package connectorcli

import (
	"bytes"
	"context"
	"csgclaw/internal/processgroup"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type OperationError struct{ Code string }

func (e *OperationError) Error() string { return e.Code }
func OperationFailure(output []byte) error {
	value := strings.ToLower(string(output))
	for _, item := range []struct{ match, code string }{{"access_denied", "authorization_denied"}, {"authorization was denied", "authorization_denied"}, {"authorization denied", "authorization_denied"}, {"user denied", "authorization_denied"}, {"expired_token", "authorization_expired"}, {"device code has expired", "authorization_expired"}, {"device code expired", "authorization_expired"}, {"authorization expired", "authorization_expired"}, {"permission denied", "permission_denied"}, {"insufficient_scope", "permission_denied"}} {
		if strings.Contains(value, item.match) {
			return &OperationError{Code: item.code}
		}
	}
	return &OperationError{Code: "cli_operation_failed"}
}

type ProcessRef struct {
	PID     int    `json:"pid"`
	Started string `json:"started"`
}
type Runner func(context.Context, Client, io.Reader, ...string) ([]byte, error)

type Client struct {
	Runner               Runner
	Executable           string
	ConfigDir            string
	OperationID          string
	Process              func(ProcessRef) error
	URL                  func(string)
	Env                  map[string]string
	ExcludeEnv           func(string) bool
	AuthorizationDomains []string
}

func (c Client) Environment() []string {
	result := make([]string, 0, len(os.Environ())+len(c.Env)+1)
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		_, overridden := c.Env[key]
		if ok && !overridden && (c.ExcludeEnv == nil || !c.ExcludeEnv(key)) && key != "CSGCLAW_CLI_OPERATION" {
			result = append(result, item)
		}
	}
	for key, value := range c.Env {
		result = append(result, key+"="+value)
	}
	return append(result, "CSGCLAW_CLI_OPERATION="+c.OperationID)
}

var urlPattern = regexp.MustCompile(`https://[^\s<>"'\x1b]+`)

func AllowedAuthorizationURL(raw string, domains []string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

type commandOutput struct {
	mu      sync.Mutex
	data    bytes.Buffer
	domains []string
	notify  func(string)
	last    string
}

func (w *commandOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.data.Len()+len(data) > 1<<20 {
		return 0, errors.New("CLI output exceeded limit")
	}
	_, _ = w.data.Write(data)
	if w.notify != nil {
		for _, raw := range urlPattern.FindAllString(w.data.String(), -1) {
			raw = strings.TrimRight(raw, ".,);]")
			if AllowedAuthorizationURL(raw, w.domains) && raw != w.last {
				w.last = raw
				w.notify(raw)
			}
		}
	}
	return len(data), nil
}
func (c Client) Run(ctx context.Context, input io.Reader, args ...string) ([]byte, error) {
	if c.Runner != nil {
		return c.Runner(ctx, c, input, args...)
	}
	if err := os.MkdirAll(c.ConfigDir, 0700); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, c.Executable, args...)
	cmd.Env = c.Environment()
	cmd.Dir = c.ConfigDir
	cmd.Stdin = input
	configureCommand(cmd)
	output := &commandOutput{notify: c.URL, domains: c.AuthorizationDomains}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error { return stopCommand(cmd.Process.Pid) }
	if err := processgroup.Start(cmd); err != nil {
		return nil, errors.New("could not start CLI")
	}
	ref := ProcessRef{PID: cmd.Process.Pid, Started: processStarted(cmd.Process.Pid)}
	if c.Process != nil {
		if err := c.Process(ref); err != nil {
			_ = stopCommand(ref.PID)
			_ = cmd.Wait()
			return nil, err
		}
	}
	err := cmd.Wait()
	cleanupErr := stopCommand(ref.PID)
	if cleanupErr != nil {
		return nil, cleanupErr
	}
	if c.Process != nil {
		if saveErr := c.Process(ProcessRef{}); saveErr != nil {
			return nil, saveErr
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return output.data.Bytes(), OperationFailure(output.data.Bytes())
	}
	return output.data.Bytes(), nil
}
func AtomicWrite(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(filename), ".cli-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filename)
}

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"csgclaw/internal/connectorcli"
	projection "csgclaw/internal/runtimeextension/connectorcli"
)

func cliReady(r record) bool { return r.CLI != nil && r.CLI.Ready }
func cliPayload(agentID string, r record) *CLIProjection {
	p := &CLIProjection{Payload: projection.Payload{AgentID: agentID, ResourceID: r.InstallationID, ConnectorID: r.AppID, ConfigDir: r.CLI.ConfigDir, Executable: r.CLI.Executable}, Status: r.AuthStatus}
	p.Environment = r.Package.CLI.Env(r.CLI.ConfigDir)
	p.VersionArgs = r.Package.CLI.Install.VersionArgs
	return p
}
func (s *Service) cliClient(id string, r record) connectorcli.Client {
	spec := r.Package.CLI
	client := connectorcli.Client{Executable: r.CLI.Executable, ConfigDir: r.CLI.ConfigDir, OperationID: r.CLI.Operation.ID, Env: spec.Env(r.CLI.ConfigDir), AuthorizationDomains: spec.AuthorizationDomains, ExcludeEnv: spec.ExcludesEnvironment,
		Process: func(ref connectorcli.ProcessRef) error {
			return s.editCLI(id, func(r *record) error { r.CLI.Operation.Process = ref; return nil })
		},
		URL: func(url string) { _ = s.editCLI(id, func(r *record) error { r.CLI.Operation.URL = url; return nil }) }}
	if s.options.CLI != nil {
		client.Runner = s.options.CLI.Run
	}
	return client
}
func (s *Service) checkCLIStatus(ctx context.Context, id string, r record) (bool, json.RawMessage, error) {
	client := s.cliClient(id, r)
	client.URL = nil
	data, err := client.Run(ctx, nil, r.Package.CLI.Status...)
	if err != nil {
		if ctx.Err() == nil && r.Package.CLI.StatusUnavailable(data) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return r.Package.CLI.ReadStatus(data)
}
func (s *Service) authorizeCLI(ctx context.Context, id string, r record) (bool, json.RawMessage, error) {
	spec := r.Package.CLI
	stage := func(value string) error {
		return s.editCLI(id, func(r *record) error { r.CLI.Operation.Stage = value; r.CLI.Operation.URL = ""; return nil })
	}
	workflow := connectorcli.Workflow{Client: s.cliClient(id, r), RequestedScopes: spec.RequestedScopes, Device: r.CLI.Operation.Device, Stage: stage,
		StageCompleted: func(ctx context.Context) error {
			_, status, err := s.checkCLIStatus(ctx, id, r)
			if err != nil {
				return err
			}
			return s.editCLI(id, func(r *record) error {
				if r.CLI.Operation.Cancelled || ctx.Err() != nil {
					return context.Canceled
				}
				// Publish observed identities while the operation retains ownership of readiness and worker updates.
				r.AuthStatus = status
				r.RequestedScopes = spec.RequestedScopes
				r.CheckedAt = time.Now().UTC()
				r.CLI.Operation.URL = ""
				return nil
			})
		},
		SaveDevice: func(device *connectorcli.DeviceAuthorization) error {
			return s.editCLI(id, func(r *record) error {
				r.CLI.Operation.Device = device
				if device != nil {
					r.CLI.Operation.URL = device.URL
					r.CLI.Operation.ExpiresAt = device.ExpiresAt
				} else {
					r.CLI.Operation.ExpiresAt = time.Time{}
					r.CLI.Operation.URL = ""
				}
				return nil
			})
		}}
	switch r.CLI.Operation.Action {
	case "disconnect", "logout":
		if r.CLI.Executable == "" || r.Disconnected {
			return false, nil, nil
		}
		if err := stage("revoking_authorization"); err != nil {
			return false, nil, err
		}
		if err := workflow.Run(ctx, spec.UnAuth); err != nil {
			return false, nil, err
		}
		if r.CLI.Operation.Action == "disconnect" {
			if err := stage("removing_configuration"); err != nil {
				return false, nil, err
			}
			if err := workflow.Run(ctx, spec.Disconnect); err != nil {
				return false, nil, err
			}
			return false, nil, nil
		}
	case "connect", "login":
		if err := stage("authorization"); err != nil {
			return false, nil, err
		}
		if err := workflow.Run(ctx, spec.Init); err != nil {
			return false, nil, err
		}
		if err := workflow.Run(ctx, spec.Auth); err != nil {
			return false, nil, err
		}
	}
	if err := stage("checking_authorization"); err != nil {
		return false, nil, err
	}
	ready, status, err := s.checkCLIStatus(ctx, id, r)
	if err != nil {
		return false, nil, err
	}
	if !ready && r.CLI.Operation.Action != "check" && r.CLI.Operation.Action != "logout" {
		return false, status, errors.New("CLI authorization is unavailable")
	}
	return ready, status, nil
}
func (s *Service) recoverCLIStatus(ctx context.Context, id string) error {
	r, err := s.cliSnapshot(id)
	if err != nil {
		return err
	}
	if !r.CLI.Operation.CommandsStarted {
		return nil
	}
	ready, status, err := s.checkCLIStatus(ctx, id, r)
	if err != nil {
		ready = false
		status = nil
	}
	return s.editCLI(id, func(r *record) error {
		if r.CLI.Ready != ready || !bytes.Equal(r.AuthStatus, status) {
			r.Revision++
		}
		r.CLI.Ready = ready
		r.AuthStatus = status
		r.RequestedScopes = r.Package.CLI.RequestedScopes
		if ready {
			r.Disconnected = false
		}
		r.CheckedAt = time.Now().UTC()
		return nil
	})
}
func (s *Service) CLIExtensionName(resourceID string) (string, error) {
	r, err := s.cliSnapshot(resourceID)
	if err != nil {
		return "", err
	}
	// Preserve the extension name used by saved Feishu channel bindings.
	if r.AppID == "feishu" {
		return "feishu-lark-cli", nil
	}
	return "connector-" + r.InstallationID, nil
}

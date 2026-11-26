package relay

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"csgclaw/internal/apiclient"
	"csgclaw/internal/apitypes"
)

// LocalExecutor uses the existing Session API -> Agent Engine -> Runtime path.
// Only this explicitly selected local agent is exposed; local API credentials
// are never sent to the relay.
type LocalExecutor struct {
	Client          *apiclient.Client
	AgentID         string
	Scope           string
	stopUnconfirmed atomic.Bool
}

func (e *LocalExecutor) Ready(ctx context.Context) bool {
	if e.stopUnconfirmed.Load() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	a, err := e.Client.GetAgent(ctx, e.AgentID)
	return err == nil && a.Status == "running" && !a.Runtime.SandboxEnabled && (a.Runtime.Name == "codex" || a.Runtime.Name == "dsh") && (a.Runtime.Availability == nil || a.Runtime.Availability.State == "ready" || a.Runtime.Availability.State == "not_applicable")
}

func (e *LocalExecutor) Execute(ctx context.Context, frame Frame) Frame {
	result := Frame{Type: "response.failed", ResponseID: frame.ResponseID, Sequence: 1}
	// The executor journal checks the parent against the durable local history
	// tip before dispatch. Direct handlers must not silently ignore a parent.
	if frame.PreviousResponseID != "" && !frame.parentValidated {
		result.Error = &ProtocolError{Code: "relay_branch_unsupported", Message: "continuation parent has not been reconciled with local history"}
		return result
	}
	text, err := InputText(frame.Input)
	if err != nil && len(frame.InputFiles) == 0 && len(frame.ResourceRefs) == 0 {
		result.Error = &ProtocolError{Code: "invalid_request", Message: err.Error()}
		return result
	}
	// Hash the relay origin, published agent and session to avoid collisions with
	// interactive sessions and sessions belonging to other relay instances.
	session := fmt.Sprintf("relay-%x", sha256.Sum256([]byte(e.Scope+"\x00"+frame.SessionID)))
	var response Response
	base := "/api/v1/agents/" + url.PathEscape(e.AgentID) + "/sessions/" + session
	path := base + "/responses"
	payload := map[string]any{"input": text, "stream": false}
	if len(frame.ResourceRefs) > 0 {
		payload["resource_refs"] = frame.ResourceRefs
	}
	if len(frame.InputFiles) > 0 {
		ids, err := e.prepareInputs(ctx, frame, base)
		if err != nil {
			result.Error = &ProtocolError{Code: "input_file_unavailable", Message: "could not prepare Relay input attachments"}
			return result
		}
		payload["attachment_ids"] = ids
	}
	if frame.start != nil {
		var cancel context.CancelFunc
		ctx, cancel = frame.start()
		defer cancel()
	}
	err = e.Client.DoJSON(ctx, http.MethodPost, path, payload, &response)
	if ctx.Err() != nil {
		// Closing an HTTP request alone is not proof that runtime cleanup has
		// finished. The Session cancel endpoint waits for Agent Engine cleanup.
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if stopErr := e.Client.DoNoContent(stopCtx, http.MethodPost, path+"/cancel"); stopErr != nil {
			e.stopUnconfirmed.Store(true)
			result.Error = &ProtocolError{Code: "cancellation_unconfirmed", Message: "local execution stop could not be confirmed; executor is not ready"}
			return result
		}
		result.Type = "response.cancelled"
		return result
	}
	if err != nil {
		result.Error = &ProtocolError{Code: "execution_failed", Message: "local Session API execution failed"}
		var apiError *apiclient.APIError
		if errors.As(err, &apiError) {
			switch apiError.Code {
			case "resource_not_attached", "resource_disabled", "resource_unavailable", "invalid_resource_reference":
				result.Error = &ProtocolError{Code: apiError.Code, Message: "The selected resource is unavailable. Remove or reselect it before sending again."}
			}
		}
		// Translate only this locally generated diagnostic. Never relay arbitrary
		// upstream error text, which may contain credentials or private inputs.
		if strings.Contains(err.Error(), "this request contains Responses tool definitions or tool-use history") && strings.Contains(err.Error(), "native Responses tool support") {
			result.Error = &ProtocolError{Code: "model_protocol_unsupported", Message: "the published agent requires a provider/model with native Responses tool support"}
		}
		return result
	}
	if response.Status != "completed" {
		result.Error = &ProtocolError{Code: "execution_failed", Message: "local execution did not complete"}
		return result
	}
	var attachments []apitypes.MessageAttachment
	for _, output := range response.Output {
		attachments = append(attachments, output.Attachments...)
	}
	if len(attachments) > 0 {
		if frame.deliver != nil {
			return frame.deliver(response.Text(), attachments)
		}
		result.Error = &ProtocolError{Code: "file_delivery_failed", Message: "Relay output delivery is unavailable"}
		return result
	}
	result.Type = "response.completed"
	result.OutputText = response.Text()
	return result
}

func safeID(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/\\?#\x00\r\n")
}

func (e *LocalExecutor) prepareInputs(ctx context.Context, frame Frame, base string) ([]string, error) {
	if frame.fileClient == nil || len(frame.InputFiles) > 10 {
		return nil, fmt.Errorf("Relay input transfer unavailable or attachment count exceeded")
	}
	var ids []string
	for _, input := range frame.InputFiles {
		id, err := func() (string, error) {
			file, err := os.CreateTemp("", "csgclaw-relay-input-*")
			if err != nil {
				return "", err
			}
			defer os.Remove(file.Name())
			defer file.Close()
			if err := retryFileTransfer(ctx, func() error {
				if err := file.Truncate(0); err != nil {
					return err
				}
				if _, err := file.Seek(0, io.SeekStart); err != nil {
					return err
				}
				return frame.fileClient.downloadFile(ctx, frame.fileScope, input, file)
			}); err != nil {
				return "", err
			}
			attachments, err := e.Client.UploadSessionFile(ctx, base+"/attachments", input.Name, input.MediaType, file)
			if err != nil {
				return "", err
			}
			if len(attachments) != 1 {
				return "", fmt.Errorf("Session did not confirm attachment")
			}
			return attachments[0].ID, nil
		}()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

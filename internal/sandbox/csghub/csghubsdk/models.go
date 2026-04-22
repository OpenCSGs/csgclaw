package csghubsdk

import "time"

// VolumeSpec binds a PVC subpath to a mount path inside the sandbox workload.
// Mirrors pycsghub RunnerVolumeSpec / Starhub types.SandboxVolume.
type VolumeSpec struct {
	SandboxMountSubpath string `json:"sandbox_mount_subpath"`
	SandboxMountPath    string `json:"sandbox_mount_path"`
	ReadOnly            bool   `json:"read_only,omitempty"`
}

// CreateRequest is the body for POST /api/v1/sandboxes.
//
// Fields match Starhub's types.SandboxCreateRequest; the server assigns a
// UUID that is never exposed on the wire. ResourceID defaults to 77 when
// omitted by the server (parity with the Python client, which sets the same
// default); we leave Go's zero value (0) so callers can decide explicitly.
type CreateRequest struct {
	Image        string            `json:"image"`
	ClusterID    string            `json:"cluster_id,omitempty"`
	ResourceID   int               `json:"resource_id,omitempty"`
	SandboxName  string            `json:"sandbox_name"`
	Environments map[string]string `json:"environments,omitempty"`
	Volumes      []VolumeSpec      `json:"volumes,omitempty"`
	Port         int               `json:"port,omitempty"`
	Timeout      int               `json:"timeout,omitempty"`
}

// UpdateConfigRequest is the body for PATCH /api/v1/sandboxes/{id}.
// Fields mirror Starhub's types.SandboxUpdateConfigRequest.
type UpdateConfigRequest struct {
	ResourceID   int               `json:"resource_id,omitempty"`
	Image        string            `json:"image,omitempty"`
	Environments map[string]string `json:"environments,omitempty"`
	Volumes      []VolumeSpec      `json:"volumes,omitempty"`
	Port         int               `json:"port,omitempty"`
	Timeout      int               `json:"timeout,omitempty"`
}

// CreateResponseSpec is the spec snapshot embedded in Response.
type CreateResponseSpec struct {
	SandboxName  string            `json:"sandbox_name"`
	Image        string            `json:"image"`
	Environments map[string]string `json:"environments,omitempty"`
	Volumes      []VolumeSpec      `json:"volumes,omitempty"`
	Port         int               `json:"port,omitempty"`
}

// SandboxState is the runtime status for a sandbox workload.
type SandboxState struct {
	Status     string    `json:"status"`
	ExitedCode int       `json:"exited_code,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	Timeout    int       `json:"timeout,omitempty"`
}

// Response is the envelope returned by lifecycle APIs.
type Response struct {
	Spec  CreateResponseSpec `json:"spec"`
	State SandboxState       `json:"state"`
}

// UploadFileResponse is the JSON body from POST .../v1/sandboxes/{name}/upload.
type UploadFileResponse struct {
	Message string `json:"message"`
}

// errorBody mirrors SandboxErrorResponse: Starhub and gateways emit any of
// {message, msg, error} plus an optional numeric code. Only used internally
// for error formatting.
type errorBody struct {
	Code    *int   `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Msg     string `json:"msg,omitempty"`
}

func (e errorBody) line() string {
	if e.Error != "" {
		return e.Error
	}
	if e.Message != "" {
		if e.Code != nil {
			return formatCodeMessage(*e.Code, e.Message)
		}
		return e.Message
	}
	if e.Msg != "" {
		if e.Code != nil {
			return formatCodeMessage(*e.Code, e.Msg)
		}
		return e.Msg
	}
	return "unknown error"
}

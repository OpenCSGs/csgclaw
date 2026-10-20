package apps

import (
	"context"
	"encoding/json"
	"time"

	"csgclaw/internal/connectorcli"
	projection "csgclaw/internal/runtimeextension/connectorcli"
	skill "csgclaw/internal/skill/state"
)

const KindMCP = "mcp"
const KindCLI = "cli"

type CLIProjection struct {
	projection.Payload
	Status json.RawMessage
}

type CLIOptions struct {
	LegacySecret func(context.Context, string, string) (string, error)
	Install      func(context.Context, string, func(string)) (connectorcli.Package, error)
	Run          connectorcli.Runner
}

type CLIRequest struct {
	OperationID string `json:"operation_id"`
	Revision    uint64 `json:"resource_revision"`
	Action      string `json:"-"`
}

type CLIOperation struct {
	ID           string    `json:"operation_id"`
	SessionID    string    `json:"session_id"`
	ResourceID   string    `json:"resource_id"`
	Revision     uint64    `json:"resource_revision"`
	Action       string    `json:"action"`
	Stage        string    `json:"stage"`
	Status       string    `json:"status"`
	URL          string    `json:"authorization_url,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	FailureStage string    `json:"failure_stage,omitempty"`
	ErrorCode    string    `json:"error_code,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type cliOperation struct {
	CLIOperation
	CommandsStarted bool                              `json:"commands_started,omitempty"`
	Process         connectorcli.ProcessRef           `json:"process,omitempty"`
	Device          *connectorcli.DeviceAuthorization `json:"device,omitempty"`
	Workers         map[string]bool                   `json:"workers,omitempty"`
	Committed       bool                              `json:"committed,omitempty"`
	Settled         bool                              `json:"settled,omitempty"`
	Cancelled       bool                              `json:"cancelled,omitempty"`
}

type cliRecord struct {
	Ready      bool                    `json:"ready"`
	ConfigDir  string                  `json:"config_dir"`
	Executable string                  `json:"executable"`
	SkillsDir  string                  `json:"skills_dir"`
	Queued     *cliOperation           `json:"queued,omitempty"`
	Recovery   *cliOperation           `json:"recovery,omitempty"`
	Operation  *cliOperation           `json:"operation,omitempty"`
	Completed  map[string]CLIOperation `json:"completed,omitempty"`
}

type cliBinding struct {
	Remove          bool             `json:"remove,omitempty"`
	PlannedEnabled  bool             `json:"planned_enabled"`
	PlannedRevision uint64           `json:"planned_revision"`
	Pending         bool             `json:"pending"`
	Resume          bool             `json:"resume"`
	ResumeRecorded  bool             `json:"resume_recorded"`
	Plan            *skill.SkillPlan `json:"skill_plan,omitempty"`
}

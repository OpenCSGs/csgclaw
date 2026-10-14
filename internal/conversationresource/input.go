package conversationresource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"csgclaw/internal/agentengine/contract"
	"csgclaw/internal/apitypes"
	"csgclaw/internal/resourcequery"
	"csgclaw/internal/runtime/instructions"
	"csgclaw/internal/slashcommand"
)

const MaxReferences = 16

const (
	InvalidReference    = "invalid_resource_reference"
	ResourceNotAttached = "resource_not_attached"
	ResourceDisabled    = "resource_disabled"
	ResourceUnavailable = "resource_unavailable"
	ServiceUnavailable  = "resource_service_unavailable"
	InvalidRequest      = "invalid_request"
)

type Error struct {
	Code      string
	Reference apitypes.ResourceReference
	Cause     error
}

func (e *Error) Error() string {
	if e.Reference.ID == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s:%s", e.Code, e.Reference.Source, e.Reference.ID)
}

func (e *Error) Unwrap() error { return e.Cause }

func readError(err error) error {
	if err == nil {
		return nil
	}
	var failure *Error
	if errors.As(err, &failure) {
		return err
	}
	if contract.ErrorCodeOf(err) == contract.ErrorAgentNotFound || errors.Is(err, resourcequery.ErrInvalid) || errors.Is(err, resourcequery.ErrChanged) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return &Error{Code: ServiceUnavailable, Cause: err}
}

var skillSlug = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type NormalizedInput struct {
	Content string
	Refs    []apitypes.ResourceReference
}

func Normalize(content string, refs []apitypes.ResourceReference) (NormalizedInput, error) {
	if len(refs) > MaxReferences {
		return NormalizedInput{}, &Error{Code: InvalidReference}
	}
	result := NormalizedInput{Content: content, Refs: []apitypes.ResourceReference{}}
	seen := map[apitypes.ResourceReference]bool{}
	for _, ref := range refs {
		ref.ID = strings.TrimSpace(ref.ID)
		if ref.ID == "" || strings.ContainsAny(ref.ID, "\r\n\x00") {
			return NormalizedInput{}, &Error{Code: InvalidReference, Reference: ref}
		}
		switch ref.Source {
		case apitypes.ResourceSourceSkill:
			if ref.ID == "." || ref.ID == ".." || !skillSlug.MatchString(ref.ID) {
				return NormalizedInput{}, &Error{Code: InvalidReference, Reference: ref}
			}
		case apitypes.ResourceSourceMCP, apitypes.ResourceSourceApp:
		default:
			return NormalizedInput{}, &Error{Code: InvalidReference, Reference: ref}
		}
		if !seen[ref] {
			seen[ref] = true
			result.Refs = append(result.Refs, ref)
		}
	}
	return result, nil
}

// NormalizeLegacy 将已有客户端的技能输入转换到公共引用格式。
func NormalizeLegacy(content string, refs []apitypes.ResourceReference) (NormalizedInput, error) {
	cmd, ok, err := slashcommand.Parse(content)
	if err != nil {
		return NormalizedInput{}, &Error{Code: InvalidReference}
	}
	if len(refs) > 0 && ok && slashcommand.IsNewConversationCommand(cmd) {
		return NormalizedInput{}, &Error{Code: InvalidRequest}
	}
	if ok && cmd.Name == slashcommand.UseSkillCommandName {
		refs = append(append([]apitypes.ResourceReference(nil), refs...), apitypes.ResourceReference{Source: apitypes.ResourceSourceSkill, ID: cmd.Arg})
		content = cmd.Body
	}
	return Normalize(content, refs)
}

type ResolvedResource struct {
	apitypes.ResourceSnapshot
	ToolNames []string `json:"tool_names,omitempty"`
}
type ResolvedSelection struct {
	AgentID   string             `json:"-"`
	Version   int                `json:"version"`
	Resources []ResolvedResource `json:"resources"`
}

func (s ResolvedSelection) Snapshot() *apitypes.ResourceSelection {
	result := &apitypes.ResourceSelection{Version: 1, AgentID: s.AgentID, Refs: []apitypes.ResourceSnapshot{}}
	for _, item := range s.Resources {
		result.Refs = append(result.Refs, item.ResourceSnapshot)
	}
	return result
}

func Render(selection ResolvedSelection) (string, error) {
	if selection.Resources == nil {
		selection.Resources = []ResolvedResource{}
	}
	selection.Version = 1
	data, err := json.Marshal(selection)
	if err != nil {
		return "", err
	}
	return instructions.ResourceSelectionInstructions + "\n\n本次消息的资源选择（JSON 数据）：\n" + string(data), nil
}

func (s *Service) PrepareInput(ctx context.Context, agentID, content string, selection *apitypes.ResourceSelection) (string, string, error) {
	refs := []apitypes.ResourceReference{}
	if selection != nil {
		if selection.Version != 1 || selection.AgentID != agentID {
			return "", "", &Error{Code: ResourceUnavailable}
		}
		for _, ref := range selection.Refs {
			refs = append(refs, ref.ResourceReference)
		}
	}
	input, err := NormalizeLegacy(content, refs)
	if err != nil {
		return "", "", err
	}
	resolved, err := s.Resolve(ctx, agentID, input.Refs)
	if err != nil {
		return "", "", err
	}
	prompt, err := Render(resolved)
	return input.Content, prompt, err
}

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InstructionsDocument struct {
	Instructions string `json:"instructions"`
	Effective    string `json:"effective"`
}

func (s *Service) InstructionsDocument(id string) (InstructionsDocument, error) {
	got, ok := s.Agent(id)
	if !ok {
		return InstructionsDocument{}, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}
	layout, err := s.agentLayout(got.ID, got.RuntimeKind)
	if err != nil {
		return InstructionsDocument{}, err
	}
	if strings.TrimSpace(layout.InstructionsPath) == "" {
		return InstructionsDocument{}, fmt.Errorf("runtime %q does not expose an instructions document", got.RuntimeKind)
	}
	effective, err := os.ReadFile(layout.InstructionsPath)
	if err != nil && !os.IsNotExist(err) {
		return InstructionsDocument{}, fmt.Errorf("read effective agent instructions: %w", err)
	}
	return InstructionsDocument{Instructions: got.Instructions, Effective: string(effective)}, nil
}

func (s *Service) UpdateEffectiveInstructions(ctx context.Context, id, effective string) (InstructionsDocument, error) {
	got, ok := s.Agent(id)
	if !ok {
		return InstructionsDocument{}, fmt.Errorf("agent %q not found", strings.TrimSpace(id))
	}
	layout, err := s.agentLayout(got.ID, got.RuntimeKind)
	if err != nil {
		return InstructionsDocument{}, err
	}
	if strings.TrimSpace(layout.InstructionsPath) == "" {
		return InstructionsDocument{}, fmt.Errorf("runtime %q does not expose an instructions document", got.RuntimeKind)
	}
	if err := os.MkdirAll(filepath.Dir(layout.InstructionsPath), 0o755); err != nil {
		return InstructionsDocument{}, err
	}
	if err := os.WriteFile(layout.InstructionsPath, []byte(strings.TrimRight(effective, "\n")+"\n"), 0o644); err != nil {
		return InstructionsDocument{}, fmt.Errorf("write effective agent instructions: %w", err)
	}
	instructions := ExtractUserInstructionsFromAgentsDocument(effective)
	if _, err := s.Update(ctx, id, UpdateRequest{Instructions: &instructions, FieldMask: []string{"instructions"}}); err != nil {
		return InstructionsDocument{}, err
	}
	return s.InstructionsDocument(id)
}

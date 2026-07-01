package team

import "strings"

type CreateTeamWithMembersInput struct {
	Title          string
	LeadAgentID    string
	MemberAgentIDs []string
}

func (s *Service) CreateTeamWithMembers(input CreateTeamWithMembersInput) (TeamMeta, error) {
	memberAgentIDs, err := uniqueAgentIDs(input.MemberAgentIDs)
	if err != nil {
		return TeamMeta{}, err
	}
	leadAgentID, err := requireAgentID("lead_agent_id", input.LeadAgentID)
	if err != nil {
		return TeamMeta{}, err
	}

	return s.CreateTeam(CreateTeamInput{
		Title:          strings.TrimSpace(input.Title),
		LeadAgentID:    leadAgentID,
		MemberAgentIDs: memberAgentIDs,
	})
}

func uniqueTrimmedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func uniqueParticipantIDs(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value, err := requireCanonicalParticipantID("participant_id", value)
		if err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}

func uniqueAgentIDs(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value, err := requireAgentID("agent_id", value)
		if err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}

func withoutAgentID(values []string, omit string) []string {
	omit = strings.TrimSpace(omit)
	if omit == "" || len(values) == 0 {
		return values
	}
	out := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value) == omit {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

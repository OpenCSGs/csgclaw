package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"csgclaw/internal/agent"
	"csgclaw/internal/team"
)

const managerPlannerModel = "csgclaw-manager-planner"

type teamPlanMember struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Role        string `json:"role,omitempty"`
	Description string `json:"description,omitempty"`
}

type managerPlanContext struct {
	TeamID              string                 `json:"team_id"`
	RoomID              string                 `json:"room_id"`
	LeadBotID           string                 `json:"lead_bot_id"`
	AssignableMemberIDs []string               `json:"assignable_member_ids"`
	Members             []teamPlanMember       `json:"members"`
	Task                managerPlanTaskContext `json:"task"`
}

type managerPlanTaskContext struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	AssignedTo string `json:"assigned_to,omitempty"`
	Priority   int    `json:"priority,omitempty"`
}

type managerPlanLLMResponse struct {
	PlanSummary string               `json:"plan_summary"`
	Tasks       []managerPlanLLMTask `json:"tasks"`
}

type managerPlanLLMTask struct {
	IDRef          string           `json:"id_ref"`
	Title          string           `json:"title"`
	Body           string           `json:"body"`
	Goal           string           `json:"goal"`
	AssigneeReason string           `json:"assignee_reason"`
	Deliverable    string           `json:"deliverable"`
	AssignTo       string           `json:"assign_to"`
	DependsOnRefs  []string         `json:"depends_on_refs"`
	Priority       flexiblePriority `json:"priority"`
}

type flexiblePriority int

func (v *flexiblePriority) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		*v = 0
		return nil
	}

	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*v = flexiblePriority(n)
		return nil
	}

	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		*v = flexiblePriority(int(f))
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		*v = 0
		return nil
	}
	*v = flexiblePriority(parsePriorityValue(s))
	return nil
}

func parsePriorityValue(value string) int {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return 0
	}
	if parsed, err := strconv.Atoi(normalized); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(normalized, 64); err == nil {
		return int(parsed)
	}
	switch strings.NewReplacer(" ", "", "-", "", "_", "").Replace(normalized) {
	case "critical", "urgent", "highest", "high":
		return 9
	case "medium", "moderate", "normal", "default":
		return 5
	case "low", "lowest":
		return 1
	case "p0":
		return 10
	case "p1":
		return 9
	case "p2":
		return 5
	case "p3":
		return 3
	case "p4":
		return 1
	default:
		return 0
	}
}

func (h *Handler) managerPlanTask(ctx context.Context, meta team.TeamMeta, parent team.TeamTask) (team.PlanTaskInput, error) {
	if h == nil || h.llm == nil {
		return team.PlanTaskInput{}, &teamPlannerHTTPError{status: http.StatusServiceUnavailable, message: "llm bridge is not configured"}
	}
	planCtx := h.managerPlanContext(meta, parent)
	body, err := marshalManagerPlanPrompt(planCtx)
	if err != nil {
		return team.PlanTaskInput{}, err
	}
	respBody, status, _, err := h.llm.ChatCompletions(ctx, meta.LeadBotID, body)
	if err != nil {
		return team.PlanTaskInput{}, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return team.PlanTaskInput{}, fmt.Errorf("manager planner returned status %d: %s", status, truncatePlannerText(string(respBody), 240))
	}
	plan, err := parseManagerPlanCompletion(respBody)
	if err != nil {
		return team.PlanTaskInput{}, err
	}
	return h.normalizeManagerPlan(planCtx, plan)
}

func (h *Handler) managerPlanContext(meta team.TeamMeta, parent team.TeamTask) managerPlanContext {
	members := h.teamPlanMembers(meta)
	assignable := assignablePlanMemberIDs(meta, members)
	return managerPlanContext{
		TeamID:              meta.ID,
		RoomID:              team.EventRoomID(meta, &parent),
		LeadBotID:           meta.LeadBotID,
		AssignableMemberIDs: assignable,
		Members:             members,
		Task: managerPlanTaskContext{
			ID:         parent.ID,
			Title:      parent.Title,
			Body:       parent.Body,
			AssignedTo: parent.AssignedTo,
			Priority:   parent.Priority,
		},
	}
}

func (h *Handler) teamPlanMembers(meta team.TeamMeta) []teamPlanMember {
	seen := make(map[string]struct{})
	out := make([]teamPlanMember, 0)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		member := teamPlanMember{ID: id}
		if h != nil && h.im != nil {
			if user, ok := h.im.User(id); ok {
				member.Name = strings.TrimSpace(user.Name)
				member.Role = strings.TrimSpace(user.Role)
			}
		}
		if h != nil && h.svc != nil {
			if got, ok := h.svc.Agent(id); ok {
				member.Name = firstNonEmpty(strings.TrimSpace(got.Name), member.Name)
				member.Role = firstNonEmpty(strings.TrimSpace(got.Role), member.Role)
				member.Description = strings.TrimSpace(got.Description)
			}
		}
		if id == meta.LeadBotID {
			member.Role = agent.RoleManager
		}
		if member.Role == "" {
			member.Role = "member"
		}
		out = append(out, member)
	}

	add(meta.LeadBotID)
	if h != nil && h.im != nil {
		if room, ok := h.im.Room(meta.RoomID); ok {
			for _, memberID := range room.Members {
				add(memberID)
			}
		}
	}
	return out
}

func assignablePlanMemberIDs(meta team.TeamMeta, members []teamPlanMember) []string {
	out := make([]string, 0, len(members))
	for _, member := range members {
		if member.ID == "" || member.ID == meta.LeadBotID {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(member.Role))
		switch role {
		case agent.RoleWorker, agent.RoleAgent:
			out = append(out, member.ID)
		}
	}
	if len(out) == 0 && strings.TrimSpace(meta.LeadBotID) != "" {
		out = append(out, meta.LeadBotID)
	}
	return out
}

func marshalManagerPlanPrompt(planCtx managerPlanContext) ([]byte, error) {
	contextJSON, err := json.MarshalIndent(planCtx, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manager plan context: %w", err)
	}
	payload := map[string]any{
		"model":       managerPlannerModel,
		"temperature": 0.2,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": strings.Join([]string{
					"You are the CSGClaw team manager planning one parent task into executable child tasks.",
					"The parent task title is a short label; body carries the detailed goal, scope, and acceptance criteria. Plan from both together.",
					"Return only a strict JSON object with keys plan_summary and tasks.",
					"Task fields: id_ref, title, assign_to, depends_on_refs, priority, goal, assignee_reason, deliverable, body.",
					"priority must be an integer from 1 to 10; do not use labels such as high, medium, or low.",
					"Use assign_to only from assignable_member_ids.",
					"Create one child task unless different roles, capabilities, parallel work, or real dependencies justify multiple tasks.",
					"Every child task must explain the goal, why that assignee owns it, and the expected deliverable.",
				}, "\n"),
			},
			{
				"role":    "user",
				"content": string(contextJSON),
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode manager planner request: %w", err)
	}
	return body, nil
}

func parseManagerPlanCompletion(body []byte) (managerPlanLLMResponse, error) {
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &completion); err != nil {
		return managerPlanLLMResponse{}, fmt.Errorf("decode manager planner response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return managerPlanLLMResponse{}, fmt.Errorf("manager planner response contained no choices")
	}
	content := extractJSONObject(completion.Choices[0].Message.Content)
	if strings.TrimSpace(content) == "" {
		return managerPlanLLMResponse{}, fmt.Errorf("manager planner response did not contain JSON")
	}
	var plan managerPlanLLMResponse
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return managerPlanLLMResponse{}, fmt.Errorf("decode manager plan JSON: %w", err)
	}
	return plan, nil
}

func (h *Handler) normalizeManagerPlan(planCtx managerPlanContext, plan managerPlanLLMResponse) (team.PlanTaskInput, error) {
	assignable := make(map[string]struct{}, len(planCtx.AssignableMemberIDs))
	assignableMemberIDs := make([]string, 0, len(planCtx.AssignableMemberIDs))
	for _, id := range planCtx.AssignableMemberIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		assignableMemberIDs = append(assignableMemberIDs, id)
		assignable[id] = struct{}{}
	}
	defaultAssignee := ""
	if len(assignableMemberIDs) > 0 {
		defaultAssignee = assignableMemberIDs[0]
	}

	tasks := plan.Tasks
	if len(planCtx.AssignableMemberIDs) <= 1 && len(tasks) > 1 {
		tasks = []managerPlanLLMTask{collapseSingleExecutorPlan(planCtx, plan)}
	}
	if len(tasks) == 0 {
		return team.PlanTaskInput{}, fmt.Errorf("manager plan did not include any tasks")
	}

	refs := make(map[string]struct{}, len(tasks))
	items := make([]team.PlanTaskItem, 0, len(tasks))
	for i, taskItem := range tasks {
		idRef := strings.TrimSpace(taskItem.IDRef)
		if idRef == "" {
			idRef = fmt.Sprintf("task_%d", i+1)
		}
		if _, exists := refs[idRef]; exists {
			idRef = fmt.Sprintf("task_%d", i+1)
		}
		refs[idRef] = struct{}{}
		assignTo := strings.TrimSpace(taskItem.AssignTo)
		if assignTo == "" {
			assignTo = defaultAssignee
		}
		if _, ok := assignable[assignTo]; !ok {
			return team.PlanTaskInput{}, fmt.Errorf("manager plan task %q assign_to %q is not in assignable_member_ids", idRef, assignTo)
		}
		title := firstNonEmpty(strings.TrimSpace(taskItem.Title), planCtx.Task.Title, planCtx.Task.ID)
		priority := int(taskItem.Priority)
		if priority == 0 {
			priority = max(1, len(tasks)-i)
		}
		items = append(items, team.PlanTaskItem{
			IDRef:         idRef,
			Title:         title,
			Body:          renderPlanTaskBody(taskItem),
			AssignTo:      assignTo,
			DependsOnRefs: cloneValidDependsOnRefs(taskItem.DependsOnRefs, refs, idRef),
			Priority:      priority,
		})
	}
	if len(items) > 0 && len(items[0].DependsOnRefs) > 0 {
		items[0].DependsOnRefs = nil
	}

	return team.PlanTaskInput{
		TeamID:      planCtx.TeamID,
		TaskID:      planCtx.Task.ID,
		ActorID:     planCtx.LeadBotID,
		PlanSummary: firstNonEmpty(strings.TrimSpace(plan.PlanSummary), defaultManagerPlanSummary(len(items))),
		Tasks:       items,
	}, nil
}

func collapseSingleExecutorPlan(planCtx managerPlanContext, plan managerPlanLLMResponse) managerPlanLLMTask {
	assignee := ""
	if len(planCtx.AssignableMemberIDs) > 0 {
		assignee = planCtx.AssignableMemberIDs[0]
	}
	titles := make([]string, 0, len(plan.Tasks))
	for _, taskItem := range plan.Tasks {
		if title := strings.TrimSpace(taskItem.Title); title != "" {
			titles = append(titles, title)
		}
	}
	body := strings.TrimSpace(planCtx.Task.Body)
	if len(titles) > 0 {
		body = strings.TrimSpace(body + "\nPlanned notes: " + strings.Join(titles, "; "))
	}
	return managerPlanLLMTask{
		IDRef:          "execution",
		Title:          firstNonEmpty(planCtx.Task.Title, planCtx.Task.ID),
		Body:           body,
		Goal:           firstNonEmpty(planCtx.Task.Body, planCtx.Task.Title),
		AssigneeReason: "Only one assignable team member is available, so the task remains a single execution item.",
		Deliverable:    "Complete the requested parent task and report the result.",
		AssignTo:       assignee,
		Priority:       flexiblePriority(max(1, planCtx.Task.Priority)),
	}
}

func renderPlanTaskBody(item managerPlanLLMTask) string {
	lines := []string{
		"Goal: " + firstNonEmpty(strings.TrimSpace(item.Goal), strings.TrimSpace(item.Body), strings.TrimSpace(item.Title)),
		"Assignee reason: " + firstNonEmpty(strings.TrimSpace(item.AssigneeReason), "Best matched available team member."),
		"Deliverable: " + firstNonEmpty(strings.TrimSpace(item.Deliverable), "A concise completion report with the produced result."),
	}
	if body := strings.TrimSpace(item.Body); body != "" && body != strings.TrimSpace(item.Goal) {
		lines = append(lines, "Notes: "+body)
	}
	return strings.Join(lines, "\n")
}

func cloneValidDependsOnRefs(refs []string, known map[string]struct{}, self string) []string {
	out := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || ref == self {
			continue
		}
		if _, ok := known[ref]; !ok {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		out = append(out, ref)
	}
	return out
}

func defaultManagerPlanSummary(taskCount int) string {
	if taskCount <= 1 {
		return "Kept as one child task because the team has a single clear execution path."
	}
	return fmt.Sprintf("Split into %d child tasks because roles, dependencies, or delivery boundaries make parallel coordination useful.", taskCount)
}

func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSpace(strings.TrimSuffix(content, "```"))
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < start {
		return ""
	}
	return content[start : end+1]
}

func truncatePlannerText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit || limit <= 3 {
		return text
	}
	return text[:limit-3] + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type teamPlannerHTTPError struct {
	status  int
	message string
}

func (e *teamPlannerHTTPError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

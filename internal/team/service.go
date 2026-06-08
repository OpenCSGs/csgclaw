package team

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrTeamNotFound           = errors.New("team not found")
	ErrTaskNotFound           = errors.New("task not found")
	ErrTaskNoSubtasks         = errors.New("task has no subtasks")
	ErrApprovalNotFound       = errors.New("approval not found")
	ErrTaskTransitionInvalid  = errors.New("task state transition is invalid")
	ErrApprovalAlreadyHandled = errors.New("approval is not pending")
	ErrTaskNotClaimable       = errors.New("task is not claimable")
	ErrTaskDependenciesOpen   = errors.New("task dependencies are not completed")
	ErrWorkerAlreadyBusy      = errors.New("worker already has in-progress task")
	ErrTeamSelectionRequired  = errors.New("team selection is required")
)

type Service struct {
	mu             sync.Mutex
	now            func() time.Time
	store          *Store
	projector      *Projector
	staleTaskTTL   time.Duration
	nextSeq        int64
	nextTeamID     int64
	nextTaskID     int64
	nextApprovalID int64
	teams          map[string]TeamMeta
	tasks          map[string]map[string]*TeamTask
	approvals      map[string]map[string]*TeamApproval
	presence       map[string]map[string]*MemberPresence
	events         map[string][]TeamEvent
	dirtyPresence  map[string]map[string]struct{}
}

type CreateTeamInput struct {
	ID        string
	RoomID    string
	Channel   string
	Title     string
	LeadBotID string
	Status    string
}

type CreateTaskInput struct {
	TeamID     string
	ParentID   string
	Title      string
	Body       string
	CreatedBy  string
	AssignTo   string
	DependsOn  []string
	Priority   int
	DeadlineAt *time.Time
	TimeoutAt  *time.Time
}

type CreateTaskBatchInput struct {
	TeamID    string
	CreatedBy string
	Tasks     []CreateTaskBatchItem
}

type CreateTaskBatchItem struct {
	IDRef         string
	ParentID      string
	ParentRef     string
	Title         string
	Body          string
	AssignTo      string
	DependsOnRefs []string
	Priority      int
	DeadlineAt    *time.Time
	TimeoutAt     *time.Time
}

type BatchIDRef struct {
	IDRef  string
	TaskID string
}

type CreateTasksResult struct {
	Tasks  []TeamTask
	IDRefs []BatchIDRef
}

type AssignTaskInput struct {
	TeamID     string
	TaskID     string
	AssignedTo string
	ActorID    string
}

type ClaimTaskInput struct {
	TeamID string
	TaskID string
	BotID  string
}

type UpdateTaskStatusInput struct {
	TeamID  string
	TaskID  string
	ActorID string
	Status  string
	Result  string
	Error   string
	Reason  string
}

type CompleteTaskInput struct {
	TeamID  string
	TaskID  string
	ActorID string
	Result  string
}

type FailTaskInput struct {
	TeamID  string
	TaskID  string
	ActorID string
	Error   string
}

type CancelTaskInput struct {
	TeamID  string
	TaskID  string
	ActorID string
	Reason  string
}

type RequestApprovalInput struct {
	TeamID      string
	TaskID      string
	RequestedBy string
	ApproverID  string
	Kind        string
	Summary     string
	Payload     string
}

type ResolveApprovalInput struct {
	TeamID     string
	ApprovalID string
	ApproverID string
	Status     string
	Resolution string
}

type UpsertPresenceInput struct {
	TeamID        string
	BotID         string
	UserID        string
	AgentID       string
	Role          string
	State         string
	CurrentTaskID string
	Summary       string
}

type Option func(*Service)

type PlanTaskInput struct {
	TeamID      string
	TaskID      string
	ActorID     string
	PlanSummary string
	Tasks       []PlanTaskItem
}

type PlanTaskItem struct {
	IDRef         string
	Title         string
	Body          string
	AssignTo      string
	DependsOnRefs []string
	Priority      int
	DeadlineAt    *time.Time
	TimeoutAt     *time.Time
}

type PlanTaskResult struct {
	Parent TeamTask
	Tasks  []TeamTask
	// AlreadyPlanned indicates whether sub tasks already existed and no new plan was generated.
	AlreadyPlanned bool
}

type StartTaskInput struct {
	TeamID     string
	TaskID     string
	ActorID    string
	TaskRoomID string
}

type StartTaskResult struct {
	Parent         TeamTask
	ScheduledCount int
}

type BindTaskExecutionRoomInput struct {
	TeamID     string
	TaskID     string
	ActorID    string
	TaskRoomID string
}

func WithNowFunc(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func WithStore(store *Store) Option {
	return func(s *Service) {
		s.store = store
	}
}

func WithProjector(projector *Projector) Option {
	return func(s *Service) {
		s.projector = projector
	}
}

func WithStaleTaskTTL(ttl time.Duration) Option {
	return func(s *Service) {
		if ttl > 0 {
			s.staleTaskTTL = ttl
		}
	}
}

func NewService(opts ...Option) *Service {
	s := &Service{
		now:           time.Now,
		staleTaskTTL:  10 * time.Minute,
		teams:         make(map[string]TeamMeta),
		tasks:         make(map[string]map[string]*TeamTask),
		approvals:     make(map[string]map[string]*TeamApproval),
		presence:      make(map[string]map[string]*MemberPresence),
		events:        make(map[string][]TeamEvent),
		dirtyPresence: make(map[string]map[string]struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.store != nil {
		if err := s.loadStoreState(); err != nil {
			panic(err)
		}
	}
	return s
}

func (s *Service) CreateTeam(input CreateTeamInput) (TeamMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(input.RoomID) == "" {
		return TeamMeta{}, fmt.Errorf("room_id is required")
	}
	if strings.TrimSpace(input.Channel) == "" {
		return TeamMeta{}, fmt.Errorf("channel is required")
	}
	if strings.TrimSpace(input.LeadBotID) == "" {
		return TeamMeta{}, fmt.Errorf("lead_bot_id is required")
	}
	leadBotID, err := requireCanonicalParticipantID("lead_bot_id", input.LeadBotID)
	if err != nil {
		return TeamMeta{}, err
	}

	now := s.now()
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = s.nextTeamIdentifier()
	}
	if _, exists := s.teams[id]; exists {
		return TeamMeta{}, fmt.Errorf("team %q already exists", id)
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = TeamStatusActive
	}
	eventStart := len(s.events[id])
	meta := TeamMeta{
		ID:        id,
		RoomID:    strings.TrimSpace(input.RoomID),
		Channel:   strings.TrimSpace(input.Channel),
		Title:     strings.TrimSpace(input.Title),
		LeadBotID: leadBotID,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.teams[id] = meta
	s.appendEventLocked(id, TeamEvent{
		RoomID:    meta.RoomID,
		Type:      "team.created",
		ActorID:   meta.LeadBotID,
		Summary:   meta.Title,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(id, capturedTeamState{}, eventStart); err != nil {
		return TeamMeta{}, err
	}
	return cloneTeamMeta(meta), nil
}

func (s *Service) GetTeam(teamID string) (TeamMeta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, ok := s.teams[teamID]
	return cloneTeamMeta(meta), ok
}

func (s *Service) ListTeams() []TeamMeta {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]TeamMeta, 0, len(s.teams))
	for _, meta := range s.teams {
		out = append(out, cloneTeamMeta(meta))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) FindTeamByRoom(roomID string) (TeamMeta, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return TeamMeta{}, false
	}
	for _, meta := range s.teams {
		if meta.RoomID == roomID {
			return cloneTeamMeta(meta), true
		}
	}
	for teamID, tasks := range s.tasks {
		meta, ok := s.teams[teamID]
		if !ok {
			continue
		}
		for _, task := range tasks {
			if task != nil && strings.TrimSpace(task.RoomID) == roomID {
				return cloneTeamMeta(meta), true
			}
		}
	}
	return TeamMeta{}, false
}

func (s *Service) CreateTask(input CreateTaskInput) (TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, err := s.requireTeamLocked(input.TeamID)
	if err != nil {
		return TeamTask{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	if strings.TrimSpace(input.Title) == "" {
		return TeamTask{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.CreatedBy) == "" {
		return TeamTask{}, fmt.Errorf("created_by is required")
	}
	if err := s.validateParentLocked(input.TeamID, input.ParentID, nil); err != nil {
		return TeamTask{}, err
	}
	if err := s.validateDependsOnLocked(input.TeamID, input.DependsOn); err != nil {
		return TeamTask{}, err
	}
	if _, err := requireCanonicalParticipantID("assign_to", input.AssignTo); err != nil {
		return TeamTask{}, err
	}

	task := s.newTaskLocked(meta, input)
	s.tasksForTeamLocked(meta.ID)[task.ID] = task
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, task),
		Type:      "task.created",
		ActorID:   task.CreatedBy,
		TaskID:    task.ID,
		TargetID:  task.AssignedTo,
		Summary:   task.Title,
		CreatedAt: task.CreatedAt,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) CreateTasks(input CreateTaskBatchInput) (CreateTasksResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, err := s.requireTeamLocked(input.TeamID)
	if err != nil {
		return CreateTasksResult{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	if strings.TrimSpace(input.CreatedBy) == "" {
		return CreateTasksResult{}, fmt.Errorf("created_by is required")
	}
	if len(input.Tasks) == 0 {
		return CreateTasksResult{}, fmt.Errorf("tasks are required")
	}

	idRefs := make(map[string]string, len(input.Tasks))
	pendingTaskIDs := make(map[string]struct{}, len(input.Tasks))
	pending := make([]*TeamTask, 0, len(input.Tasks))
	result := CreateTasksResult{}

	predictedOffset := 0
	for i, item := range input.Tasks {
		if strings.TrimSpace(item.Title) == "" {
			return CreateTasksResult{}, fmt.Errorf("tasks[%d].title is required", i)
		}
		if _, err := requireCanonicalParticipantID("assign_to", item.AssignTo); err != nil {
			return CreateTasksResult{}, fmt.Errorf("tasks[%d].assign_to: %w", i, err)
		}
		idRef := strings.TrimSpace(item.IDRef)
		if idRef != "" {
			if _, exists := idRefs[idRef]; exists {
				return CreateTasksResult{}, fmt.Errorf("duplicate id_ref %q", idRef)
			}
			predictedID := s.peekNextTaskIdentifier(predictedOffset)
			idRefs[idRef] = predictedID
			pendingTaskIDs[predictedID] = struct{}{}
		}
		predictedOffset++
	}

	for i, item := range input.Tasks {
		dependsOn := make([]string, 0, len(item.DependsOnRefs))
		for _, ref := range item.DependsOnRefs {
			ref = strings.TrimSpace(ref)
			taskID, ok := idRefs[ref]
			if !ok {
				return CreateTasksResult{}, fmt.Errorf("tasks[%d].depends_on_refs contains unknown id_ref %q", i, ref)
			}
			dependsOn = append(dependsOn, taskID)
		}
		parentID := strings.TrimSpace(item.ParentID)
		parentRef := strings.TrimSpace(item.ParentRef)
		if parentID != "" && parentRef != "" {
			return CreateTasksResult{}, fmt.Errorf("tasks[%d] cannot set both parent_id and parent_ref", i)
		}
		if parentRef != "" {
			resolvedParentID, ok := idRefs[parentRef]
			if !ok {
				return CreateTasksResult{}, fmt.Errorf("tasks[%d].parent_ref contains unknown id_ref %q", i, parentRef)
			}
			parentID = resolvedParentID
		}
		if err := s.validateParentLocked(input.TeamID, parentID, pendingTaskIDs); err != nil {
			return CreateTasksResult{}, fmt.Errorf("tasks[%d].parent: %w", i, err)
		}
		task := s.newTaskLocked(meta, CreateTaskInput{
			TeamID:     input.TeamID,
			ParentID:   parentID,
			Title:      item.Title,
			Body:       item.Body,
			CreatedBy:  input.CreatedBy,
			AssignTo:   item.AssignTo,
			DependsOn:  dependsOn,
			Priority:   item.Priority,
			DeadlineAt: item.DeadlineAt,
			TimeoutAt:  item.TimeoutAt,
		})
		pending = append(pending, task)
		if idRef := strings.TrimSpace(item.IDRef); idRef != "" {
			result.IDRefs = append(result.IDRefs, BatchIDRef{IDRef: idRef, TaskID: task.ID})
		}
	}

	for _, task := range pending {
		s.tasksForTeamLocked(meta.ID)[task.ID] = task
		s.appendEventLocked(meta.ID, TeamEvent{
			RoomID:    EventRoomID(meta, task),
			Type:      "task.created",
			ActorID:   task.CreatedBy,
			TaskID:    task.ID,
			TargetID:  task.AssignedTo,
			Summary:   task.Title,
			CreatedAt: task.CreatedAt,
		})
		result.Tasks = append(result.Tasks, cloneTask(*task))
	}
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return CreateTasksResult{}, err
	}
	return result, nil
}

func (s *Service) PlanTask(input PlanTaskInput) (PlanTaskResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, meta, err := s.requireTaskLocked(input.TeamID, input.TaskID)
	if err != nil {
		return PlanTaskResult{}, err
	}
	if strings.TrimSpace(task.ParentID) != "" {
		return PlanTaskResult{}, fmt.Errorf("%w: only parent tasks can be planned", ErrTaskTransitionInvalid)
	}
	if !taskIsPlannable(task, meta) {
		return PlanTaskResult{}, fmt.Errorf("%w: cannot plan task in status %s", ErrTaskTransitionInvalid, task.Status)
	}
	existingChildren := s.taskChildrenLocked(meta.ID, task.ID)
	if len(existingChildren) > 0 {
		out := PlanTaskResult{
			Parent:         cloneTask(*task),
			AlreadyPlanned: true,
		}
		for _, child := range existingChildren {
			out.Tasks = append(out.Tasks, cloneTask(*child))
		}
		return out, nil
	}

	actorID := strings.TrimSpace(input.ActorID)
	if actorID == "" {
		actorID = "web"
	}
	if len(input.Tasks) == 0 {
		return PlanTaskResult{}, fmt.Errorf("plan tasks are required")
	}

	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	now := s.now()

	idRefs := make(map[string]string, len(input.Tasks))
	for i, item := range input.Tasks {
		if strings.TrimSpace(item.Title) == "" {
			return PlanTaskResult{}, fmt.Errorf("plan tasks[%d].title is required", i)
		}
		idRef := strings.TrimSpace(item.IDRef)
		if idRef == "" {
			idRef = fmt.Sprintf("task_%d", i+1)
		}
		if _, exists := idRefs[idRef]; exists {
			return PlanTaskResult{}, fmt.Errorf("plan tasks[%d].id_ref %q is duplicated", i, idRef)
		}
		idRefs[idRef] = s.peekNextTaskIdentifier(i)
	}

	resolvedDependsOn := make([][]string, len(input.Tasks))
	for i, item := range input.Tasks {
		for _, ref := range item.DependsOnRefs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			taskID, ok := idRefs[ref]
			if !ok {
				return PlanTaskResult{}, fmt.Errorf("plan tasks[%d].depends_on_refs contains unknown id_ref %q", i, ref)
			}
			resolvedDependsOn[i] = append(resolvedDependsOn[i], taskID)
		}
	}

	task.PlanSummary = strings.TrimSpace(input.PlanSummary)
	task.Status = TaskStatusPending
	task.Result = ""
	task.Error = ""
	task.CompletedAt = nil
	task.UpdatedAt = now

	result := PlanTaskResult{AlreadyPlanned: false}
	for i, item := range input.Tasks {
		assignTo := firstNonEmpty(strings.TrimSpace(item.AssignTo), fallbackPlanAssignee(task.AssignedTo, meta))
		if _, err := requireCanonicalParticipantID("assign_to", assignTo); err != nil {
			return PlanTaskResult{}, fmt.Errorf("plan tasks[%d].assign_to: %w", i, err)
		}
		child := s.newTaskLocked(meta, CreateTaskInput{
			TeamID:     meta.ID,
			ParentID:   task.ID,
			Title:      item.Title,
			Body:       item.Body,
			CreatedBy:  actorID,
			AssignTo:   assignTo,
			DependsOn:  resolvedDependsOn[i],
			Priority:   item.Priority,
			DeadlineAt: item.DeadlineAt,
			TimeoutAt:  item.TimeoutAt,
		})
		child.Status = TaskStatusPending
		child.DispatchedAt = nil
		s.tasksForTeamLocked(meta.ID)[child.ID] = child
		result.Tasks = append(result.Tasks, cloneTask(*child))
		s.appendEventLocked(meta.ID, TeamEvent{
			RoomID:    EventRoomID(meta, child),
			Type:      "task.created",
			ActorID:   child.CreatedBy,
			TaskID:    child.ID,
			TargetID:  child.AssignedTo,
			Summary:   child.Title,
			CreatedAt: child.CreatedAt,
		})
	}
	if task.PlanSummary == "" {
		task.PlanSummary = defaultPlanSummary(len(result.Tasks))
	}
	result.Parent = cloneTask(*task)
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, task),
		Type:      "task.planned",
		ActorID:   actorID,
		TaskID:    task.ID,
		Summary:   task.PlanSummary,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return PlanTaskResult{}, err
	}
	return result, nil
}

func (s *Service) BindTaskExecutionRoom(input BindTaskExecutionRoomInput) (TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, meta, err := s.requireTaskLocked(input.TeamID, input.TaskID)
	if err != nil {
		return TeamTask{}, err
	}
	if strings.TrimSpace(task.ParentID) != "" {
		return TeamTask{}, fmt.Errorf("%w: only parent tasks can have execution rooms", ErrTaskTransitionInvalid)
	}
	roomID := strings.TrimSpace(input.TaskRoomID)
	if roomID == "" {
		return TeamTask{}, fmt.Errorf("task_room_id is required")
	}
	if ExecutionRoomBound(*task, meta) {
		if strings.TrimSpace(task.RoomID) != roomID {
			return TeamTask{}, fmt.Errorf("%w: task already has execution room %s", ErrTaskTransitionInvalid, task.RoomID)
		}
		return cloneTask(*task), nil
	}

	actorID := strings.TrimSpace(input.ActorID)
	if actorID == "" {
		actorID = "web"
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	now := s.now()
	s.bindTaskExecutionRoomLocked(meta, task, roomID, now)
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    meta.RoomID,
		Type:      "task.execution_room",
		ActorID:   actorID,
		TaskID:    task.ID,
		TargetID:  roomID,
		Summary:   TaskExecutionRoomTitle(*task),
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) StartTask(input StartTaskInput) (StartTaskResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, meta, err := s.requireTaskLocked(input.TeamID, input.TaskID)
	if err != nil {
		return StartTaskResult{}, err
	}
	if strings.TrimSpace(task.ParentID) != "" {
		return StartTaskResult{}, fmt.Errorf("%w: only parent tasks can be started", ErrTaskTransitionInvalid)
	}
	if !taskStatusIsUnstarted(task.Status) {
		return StartTaskResult{}, fmt.Errorf("%w: cannot start task in status %s", ErrTaskTransitionInvalid, task.Status)
	}

	children := s.taskChildrenLocked(meta.ID, task.ID)
	if len(children) == 0 {
		return StartTaskResult{}, ErrTaskNoSubtasks
	}

	actorID := strings.TrimSpace(input.ActorID)
	if actorID == "" {
		actorID = "web"
	}

	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	now := s.now()

	scheduledCount := s.readyChildrenCountLocked(meta, task.ID)
	if scheduledCount == 0 {
		return StartTaskResult{}, fmt.Errorf("%w: no runnable subtasks under %s", ErrTaskNotClaimable, task.ID)
	}

	taskRoomID := strings.TrimSpace(input.TaskRoomID)
	if taskRoomID != "" && !ExecutionRoomBound(*task, meta) {
		s.bindTaskExecutionRoomLocked(meta, task, taskRoomID, now)
		s.appendEventLocked(meta.ID, TeamEvent{
			RoomID:    meta.RoomID,
			Type:      "task.execution_room",
			ActorID:   actorID,
			TaskID:    task.ID,
			TargetID:  taskRoomID,
			Summary:   TaskExecutionRoomTitle(*task),
			CreatedAt: now,
		})
	}

	scheduledCount = s.dispatchReadyChildrenLocked(meta, task.ID, meta.LeadBotID, now)

	task.Status = TaskStatusAssigned
	task.Result = ""
	task.Error = ""
	task.UpdatedAt = now

	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, task),
		Type:      "task.started",
		ActorID:   actorID,
		TaskID:    task.ID,
		Summary:   task.Title,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return StartTaskResult{}, err
	}
	return StartTaskResult{
		Parent:         cloneTask(*task),
		ScheduledCount: scheduledCount,
	}, nil
}

func taskStatusIsUnstarted(status string) bool {
	return strings.TrimSpace(status) == "" || strings.TrimSpace(status) == TaskStatusPending
}

func taskAssignedToManager(assignedTo string, teamMeta TeamMeta) bool {
	assignedTo = strings.TrimSpace(assignedTo)
	if assignedTo == "" {
		return false
	}
	if leadBotID := strings.TrimSpace(teamMeta.LeadBotID); leadBotID != "" {
		return ParticipantIDsMatch(assignedTo, leadBotID)
	}
	return assignedTo == "u-manager"
}

func fallbackPlanAssignee(assignedTo string, teamMeta TeamMeta) string {
	assignedTo = strings.TrimSpace(assignedTo)
	if assignedTo == "" || taskAssignedToManager(assignedTo, teamMeta) {
		return ""
	}
	return assignedTo
}

func defaultPlanSummary(taskCount int) string {
	if taskCount <= 1 {
		return "Single executable child task planned because the team has one clear execution path."
	}
	return fmt.Sprintf("%d executable child tasks planned based on team roles, dependencies, and delivery boundaries.", taskCount)
}

func taskIsPlannable(task *TeamTask, meta TeamMeta) bool {
	if task == nil {
		return false
	}
	if taskStatusIsUnstarted(task.Status) {
		return true
	}
	return strings.TrimSpace(task.Status) == TaskStatusAssigned && taskAssignedToManager(task.AssignedTo, meta)
}

func (s *Service) AssignTask(input AssignTaskInput) (TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, meta, err := s.requireTaskLocked(input.TeamID, input.TaskID)
	if err != nil {
		return TeamTask{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	if strings.TrimSpace(input.AssignedTo) == "" {
		return TeamTask{}, fmt.Errorf("assigned_to is required")
	}
	assignedTo, err := requireCanonicalParticipantID("assigned_to", input.AssignedTo)
	if err != nil {
		return TeamTask{}, err
	}
	switch task.Status {
	case TaskStatusPending, TaskStatusAssigned, TaskStatusBlocked, TaskStatusFailed:
	default:
		return TeamTask{}, fmt.Errorf("%w: cannot assign task in status %s", ErrTaskTransitionInvalid, task.Status)
	}

	now := s.now()
	task.Status = TaskStatusAssigned
	task.AssignedTo = assignedTo
	task.ClaimedBy = ""
	task.Result = ""
	task.Error = ""
	task.CompletedAt = nil
	task.UpdatedAt = now
	s.updatePresenceForTaskLocked(meta, task, PresenceStateIdle, "")
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, task),
		Type:      "task.assigned",
		ActorID:   strings.TrimSpace(input.ActorID),
		TaskID:    task.ID,
		TargetID:  task.AssignedTo,
		Summary:   task.Title,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) ClaimTask(input ClaimTaskInput) (TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, meta, err := s.requireTaskLocked(input.TeamID, input.TaskID)
	if err != nil {
		return TeamTask{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	botID, err := requireCanonicalParticipantID("bot_id", input.BotID)
	if err != nil {
		return TeamTask{}, err
	}
	if err := s.claimableLocked(meta, task, botID); err != nil {
		return TeamTask{}, err
	}
	s.claimLocked(meta, task, botID)
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) ClaimNext(teamID string, botID string) (TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	botID, err := requireCanonicalParticipantID("bot_id", botID)
	if err != nil {
		return TeamTask{}, err
	}
	if botID == "" {
		return TeamTask{}, fmt.Errorf("bot_id is required")
	}
	if strings.TrimSpace(teamID) == "" {
		return s.claimNextAcrossTeamsLocked(botID)
	}
	return s.claimNextForTeamLocked(strings.TrimSpace(teamID), botID)
}

func (s *Service) claimNextAcrossTeamsLocked(botID string) (TeamTask, error) {
	if err := s.ensureWorkerFreeGloballyLocked(botID); err != nil {
		return TeamTask{}, err
	}

	type teamCandidate struct {
		meta TeamMeta
		task *TeamTask
	}
	candidates := make([]teamCandidate, 0)
	for _, meta := range s.teams {
		if meta.Status != TeamStatusActive {
			continue
		}
		task := s.bestClaimCandidateLocked(meta.ID, botID)
		if task == nil {
			continue
		}
		candidates = append(candidates, teamCandidate{meta: meta, task: task})
	}
	if len(candidates) == 0 {
		return TeamTask{}, fmt.Errorf("%w: no task available for %s", ErrTaskNotClaimable, botID)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].task.Priority != candidates[j].task.Priority {
			return candidates[i].task.Priority > candidates[j].task.Priority
		}
		if !candidates[i].task.CreatedAt.Equal(candidates[j].task.CreatedAt) {
			return candidates[i].task.CreatedAt.Before(candidates[j].task.CreatedAt)
		}
		if candidates[i].meta.ID != candidates[j].meta.ID {
			return candidates[i].meta.ID < candidates[j].meta.ID
		}
		return candidates[i].task.ID < candidates[j].task.ID
	})

	if len(candidates) > 1 && candidates[0].task.Priority == candidates[1].task.Priority {
		return TeamTask{}, fmt.Errorf("%w: multiple teams have claimable tasks at priority %d; specify --team", ErrTeamSelectionRequired, candidates[0].task.Priority)
	}

	meta := candidates[0].meta
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	s.claimLocked(meta, candidates[0].task, botID)
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*candidates[0].task), nil
}

func (s *Service) claimNextForTeamLocked(teamID string, botID string) (TeamTask, error) {
	meta, err := s.requireTeamLocked(teamID)
	if err != nil {
		return TeamTask{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	if err := s.ensureWorkerFreeLocked(teamID, botID); err != nil {
		return TeamTask{}, err
	}

	task := s.bestClaimCandidateLocked(teamID, botID)
	if task == nil {
		return TeamTask{}, fmt.Errorf("%w: no task available for %s", ErrTaskNotClaimable, botID)
	}

	s.claimLocked(meta, task, botID)
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) bestClaimCandidateLocked(teamID string, botID string) *TeamTask {
	candidates := make([]*TeamTask, 0)
	for _, task := range s.tasksForTeamLocked(teamID) {
		if task.Status != TaskStatusPending && task.Status != TaskStatusAssigned {
			continue
		}
		if strings.TrimSpace(task.ParentID) != "" && task.DispatchedAt == nil {
			continue
		}
		if strings.TrimSpace(task.AssignedTo) != "" && !ParticipantIDsMatch(task.AssignedTo, botID) {
			continue
		}
		if !s.dependenciesCompletedLocked(teamID, task.DependsOn) {
			continue
		}
		candidates = append(candidates, task)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		if !candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
		}
		return candidates[i].ID < candidates[j].ID
	})
	return candidates[0]
}

func (s *Service) ensureWorkerFreeGloballyLocked(botID string) error {
	for teamID := range s.teams {
		if err := s.ensureWorkerFreeLocked(teamID, botID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) UpdateTaskStatus(input UpdateTaskStatusInput) (TeamTask, error) {
	switch strings.TrimSpace(input.Status) {
	case TaskStatusBlocked:
		return s.blockTask(input)
	case TaskStatusCompleted:
		return s.CompleteTask(CompleteTaskInput{
			TeamID:  input.TeamID,
			TaskID:  input.TaskID,
			ActorID: input.ActorID,
			Result:  input.Result,
		})
	case TaskStatusFailed:
		return s.FailTask(FailTaskInput{
			TeamID:  input.TeamID,
			TaskID:  input.TaskID,
			ActorID: input.ActorID,
			Error:   input.Error,
		})
	default:
		return TeamTask{}, fmt.Errorf("unsupported status update %q", input.Status)
	}
}

func (s *Service) CompleteTask(input CompleteTaskInput) (TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, meta, err := s.requireTaskLocked(input.TeamID, input.TaskID)
	if err != nil {
		return TeamTask{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	if task.Status != TaskStatusInProgress {
		return TeamTask{}, fmt.Errorf("%w: cannot complete task in status %s", ErrTaskTransitionInvalid, task.Status)
	}
	if strings.TrimSpace(input.Result) == "" {
		return TeamTask{}, fmt.Errorf("result is required")
	}
	if err := s.requireTaskOperatorLocked(meta, task, input.ActorID); err != nil {
		return TeamTask{}, err
	}
	now := s.now()
	task.Status = TaskStatusCompleted
	task.Result = strings.TrimSpace(input.Result)
	task.Error = ""
	task.UpdatedAt = now
	task.CompletedAt = timePtr(now)
	s.updatePresenceForTaskLocked(meta, task, PresenceStateIdle, "")
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, task),
		Type:      "task.completed",
		ActorID:   strings.TrimSpace(input.ActorID),
		TaskID:    task.ID,
		Summary:   task.Result,
		CreatedAt: now,
	})
	if strings.TrimSpace(task.ParentID) != "" {
		s.dispatchReadyChildrenLocked(meta, task.ParentID, meta.LeadBotID, now)
		s.maybeCompleteParentIfAllChildrenDoneLocked(meta, task.ParentID, strings.TrimSpace(input.ActorID), now)
	}
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) FailTask(input FailTaskInput) (TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, meta, err := s.requireTaskLocked(input.TeamID, input.TaskID)
	if err != nil {
		return TeamTask{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	if task.Status != TaskStatusInProgress {
		return TeamTask{}, fmt.Errorf("%w: cannot fail task in status %s", ErrTaskTransitionInvalid, task.Status)
	}
	if strings.TrimSpace(input.Error) == "" {
		return TeamTask{}, fmt.Errorf("error is required")
	}
	if err := s.requireTaskOperatorLocked(meta, task, input.ActorID); err != nil {
		return TeamTask{}, err
	}
	now := s.now()
	task.Status = TaskStatusFailed
	task.Error = strings.TrimSpace(input.Error)
	task.Result = ""
	task.CompletedAt = nil
	task.UpdatedAt = now
	s.updatePresenceForTaskLocked(meta, task, PresenceStateIdle, "")
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, task),
		Type:      "task.failed",
		ActorID:   strings.TrimSpace(input.ActorID),
		TaskID:    task.ID,
		Summary:   task.Error,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) CancelTask(input CancelTaskInput) (TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, meta, err := s.requireTaskLocked(input.TeamID, input.TaskID)
	if err != nil {
		return TeamTask{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	switch task.Status {
	case TaskStatusPending, TaskStatusAssigned, TaskStatusInProgress, TaskStatusBlocked:
	default:
		return TeamTask{}, fmt.Errorf("%w: cannot cancel task in status %s", ErrTaskTransitionInvalid, task.Status)
	}
	now := s.now()
	task.Status = TaskStatusCancelled
	if strings.TrimSpace(input.Reason) != "" {
		task.Error = strings.TrimSpace(input.Reason)
	}
	task.Result = ""
	task.CompletedAt = nil
	task.UpdatedAt = now
	s.updatePresenceForTaskLocked(meta, task, PresenceStateIdle, "")
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, task),
		Type:      "task.cancelled",
		ActorID:   strings.TrimSpace(input.ActorID),
		TaskID:    task.ID,
		Summary:   task.Error,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) RequestApproval(input RequestApprovalInput) (TeamApproval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, err := s.requireTeamLocked(input.TeamID)
	if err != nil {
		return TeamApproval{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	if strings.TrimSpace(input.RequestedBy) == "" {
		return TeamApproval{}, fmt.Errorf("requested_by is required")
	}
	if strings.TrimSpace(input.Kind) == "" {
		return TeamApproval{}, fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(input.Summary) == "" {
		return TeamApproval{}, fmt.Errorf("summary is required")
	}

	var task *TeamTask
	if strings.TrimSpace(input.TaskID) != "" {
		var taskErr error
		task, _, taskErr = s.requireTaskLocked(input.TeamID, input.TaskID)
		if taskErr != nil {
			return TeamApproval{}, taskErr
		}
	}

	now := s.now()
	approvalRoomID := meta.RoomID
	if task != nil {
		approvalRoomID = EventRoomID(meta, task)
	}
	approval := &TeamApproval{
		ID:          s.nextApprovalIdentifier(),
		TeamID:      meta.ID,
		RoomID:      approvalRoomID,
		TaskID:      strings.TrimSpace(input.TaskID),
		RequestedBy: strings.TrimSpace(input.RequestedBy),
		ApproverID:  strings.TrimSpace(input.ApproverID),
		Kind:        strings.TrimSpace(input.Kind),
		Summary:     strings.TrimSpace(input.Summary),
		Payload:     strings.TrimSpace(input.Payload),
		Status:      ApprovalStatusPending,
		CreatedAt:   now,
	}
	s.approvalsForTeamLocked(meta.ID)[approval.ID] = approval
	if task != nil && task.ClaimedBy != "" {
		s.updatePresenceLocked(meta, task.ClaimedBy, PresenceStateWaitingApproval, task.ID, approval.Summary)
	}
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    approvalRoomID,
		Type:      "approval.requested",
		ActorID:   approval.RequestedBy,
		TaskID:    approval.TaskID,
		TargetID:  approval.ID,
		Summary:   approval.Summary,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamApproval{}, err
	}
	return cloneApproval(*approval), nil
}

func (s *Service) ResolveApproval(input ResolveApprovalInput) (TeamApproval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	approval, meta, err := s.requireApprovalLocked(input.TeamID, input.ApprovalID)
	if err != nil {
		return TeamApproval{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	if approval.Status != ApprovalStatusPending {
		return TeamApproval{}, ErrApprovalAlreadyHandled
	}
	status := strings.TrimSpace(input.Status)
	switch status {
	case ApprovalStatusApproved, ApprovalStatusRejected, ApprovalStatusCancelled:
	default:
		return TeamApproval{}, fmt.Errorf("unsupported approval status %q", input.Status)
	}

	now := s.now()
	approval.Status = status
	approval.ApproverID = firstNonEmpty(strings.TrimSpace(input.ApproverID), approval.ApproverID)
	approval.Resolution = strings.TrimSpace(input.Resolution)
	approval.ResolvedAt = timePtr(now)

	var relatedTask *TeamTask
	if approval.TaskID != "" {
		task, _, taskErr := s.requireTaskLocked(input.TeamID, approval.TaskID)
		if taskErr != nil {
			return TeamApproval{}, taskErr
		}
		relatedTask = task
		if task.Status == TaskStatusBlocked {
			switch status {
			case ApprovalStatusApproved:
				task.Status = TaskStatusInProgress
				task.UpdatedAt = now
				if task.ClaimedBy != "" {
					s.updatePresenceLocked(meta, task.ClaimedBy, PresenceStateBusy, task.ID, "")
				}
			case ApprovalStatusRejected:
				task.UpdatedAt = now
				if task.ClaimedBy != "" {
					s.updatePresenceLocked(meta, task.ClaimedBy, PresenceStateBlocked, task.ID, approval.Resolution)
				}
			}
		}
	}
	approvalRoomID := meta.RoomID
	if relatedTask != nil {
		approvalRoomID = EventRoomID(meta, relatedTask)
	} else if strings.TrimSpace(approval.RoomID) != "" {
		approvalRoomID = strings.TrimSpace(approval.RoomID)
	}
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    approvalRoomID,
		Type:      "approval.resolved",
		ActorID:   approval.ApproverID,
		TaskID:    approval.TaskID,
		TargetID:  approval.ID,
		Summary:   approval.Status,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamApproval{}, err
	}
	return cloneApproval(*approval), nil
}

func (s *Service) GetTask(teamID string, taskID string) (TeamTask, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasksForTeamLocked(teamID)[taskID]
	if !ok {
		return TeamTask{}, false
	}
	return cloneTask(*task), true
}

func (s *Service) ListTasks(teamID string) []TeamTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]TeamTask, 0, len(s.tasksForTeamLocked(teamID)))
	for _, task := range s.tasksForTeamLocked(teamID) {
		out = append(out, cloneTask(*task))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) ListAllTasks() []TeamTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]TeamTask, 0)
	for _, meta := range s.teams {
		for _, task := range s.tasksForTeamLocked(meta.ID) {
			out = append(out, cloneTask(*task))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			if out[i].Priority == out[j].Priority {
				if out[i].TeamID == out[j].TeamID {
					return out[i].ID < out[j].ID
				}
				return out[i].TeamID < out[j].TeamID
			}
			return out[i].Priority > out[j].Priority
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Service) ListApprovals(teamID string) []TeamApproval {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]TeamApproval, 0, len(s.approvalsForTeamLocked(teamID)))
	for _, approval := range s.approvalsForTeamLocked(teamID) {
		out = append(out, cloneApproval(*approval))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) FindPendingApprovalByTask(teamID string, taskID string) (TeamApproval, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return TeamApproval{}, false
	}
	var latest *TeamApproval
	for _, approval := range s.approvalsForTeamLocked(teamID) {
		if approval == nil || approval.TaskID != taskID || approval.Status != ApprovalStatusPending {
			continue
		}
		if latest == nil ||
			approval.CreatedAt.After(latest.CreatedAt) ||
			(approval.CreatedAt.Equal(latest.CreatedAt) && approval.ID > latest.ID) {
			latest = approval
		}
	}
	if latest == nil {
		return TeamApproval{}, false
	}
	return cloneApproval(*latest), true
}

func (s *Service) GetPresence(teamID string, botID string) (MemberPresence, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	botID = cleanParticipantID(botID)
	if botID == "" {
		return MemberPresence{}, false
	}
	p, ok := s.presenceForTeamLocked(teamID)[botID]
	if !ok {
		return MemberPresence{}, false
	}
	return clonePresence(*p), true
}

func (s *Service) ListEvents(teamID string) []TeamEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]TeamEvent, len(s.events[teamID]))
	copy(out, s.events[teamID])
	return out
}

func (s *Service) UpsertPresence(input UpsertPresenceInput) (MemberPresence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta, err := s.requireTeamLocked(input.TeamID)
	if err != nil {
		return MemberPresence{}, err
	}
	botID, err := requireCanonicalParticipantID("bot_id", input.BotID)
	if err != nil {
		return MemberPresence{}, err
	}
	if botID == "" {
		return MemberPresence{}, fmt.Errorf("bot_id is required")
	}

	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	previous, existed := s.presenceForTeamLocked(meta.ID)[botID]
	previousSnapshot := MemberPresence{}
	if existed && previous != nil {
		previousSnapshot = clonePresence(*previous)
	}

	now := s.now()
	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = "worker"
		if ParticipantIDsMatch(botID, meta.LeadBotID) {
			role = "manager"
		}
	}
	state := strings.TrimSpace(input.State)
	if state == "" {
		state = PresenceStateIdle
	}

	p := previous
	if p == nil {
		p = &MemberPresence{
			TeamID: meta.ID,
			BotID:  botID,
		}
		s.presenceForTeamLocked(meta.ID)[botID] = p
	}
	p.UserID = strings.TrimSpace(input.UserID)
	p.AgentID = strings.TrimSpace(input.AgentID)
	p.Role = role
	p.State = state
	p.CurrentTaskID = strings.TrimSpace(input.CurrentTaskID)
	p.Summary = strings.TrimSpace(input.Summary)
	p.LastHeartbeatAt = now
	p.UpdatedAt = now
	s.markPresenceDirtyLocked(meta.ID, botID)

	if !presenceMeaningfullyChanged(previousSnapshot, *p) {
		return clonePresence(*p), nil
	}

	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    meta.RoomID,
		Type:      "presence.updated",
		ActorID:   botID,
		TaskID:    p.CurrentTaskID,
		Summary:   p.State,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return MemberPresence{}, err
	}
	return clonePresence(*p), nil
}

func (s *Service) CheckpointPresence() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store == nil {
		return nil
	}
	for teamID := range s.dirtyPresence {
		if len(s.dirtyPresence[teamID]) == 0 {
			continue
		}
		before := s.captureTeamStateLocked(teamID)
		eventStart := len(s.events[teamID])
		if err := s.persistMutationLocked(teamID, before, eventStart); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) blockTask(input UpdateTaskStatusInput) (TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, meta, err := s.requireTaskLocked(input.TeamID, input.TaskID)
	if err != nil {
		return TeamTask{}, err
	}
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	if task.Status != TaskStatusInProgress {
		return TeamTask{}, fmt.Errorf("%w: cannot block task in status %s", ErrTaskTransitionInvalid, task.Status)
	}
	if err := s.requireTaskOperatorLocked(meta, task, input.ActorID); err != nil {
		return TeamTask{}, err
	}
	if strings.TrimSpace(input.Reason) == "" {
		return TeamTask{}, fmt.Errorf("reason is required")
	}
	now := s.now()
	task.Status = TaskStatusBlocked
	task.Error = strings.TrimSpace(input.Reason)
	task.Result = ""
	task.CompletedAt = nil
	task.UpdatedAt = now
	s.updatePresenceForTaskLocked(meta, task, PresenceStateBlocked, task.Error)
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, task),
		Type:      "task.blocked",
		ActorID:   strings.TrimSpace(input.ActorID),
		TaskID:    task.ID,
		Summary:   task.Error,
		CreatedAt: now,
	})
	if err := s.persistMutationLocked(meta.ID, before, eventStart); err != nil {
		return TeamTask{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) claimableLocked(meta TeamMeta, task *TeamTask, botID string) error {
	botID = cleanParticipantID(botID)
	if botID == "" {
		return fmt.Errorf("bot_id is required")
	}
	if meta.Status != TeamStatusActive {
		return fmt.Errorf("team %q is not active", meta.ID)
	}
	switch task.Status {
	case TaskStatusPending, TaskStatusAssigned:
	default:
		return fmt.Errorf("%w: task %s is in status %s", ErrTaskNotClaimable, task.ID, task.Status)
	}
	if strings.TrimSpace(task.ParentID) != "" && task.DispatchedAt == nil {
		return fmt.Errorf("%w: task %s has not been dispatched", ErrTaskNotClaimable, task.ID)
	}
	if task.AssignedTo != "" && !ParticipantIDsMatch(task.AssignedTo, botID) {
		return fmt.Errorf("%w: task %s is assigned to %s", ErrTaskNotClaimable, task.ID, task.AssignedTo)
	}
	if !s.dependenciesCompletedLocked(meta.ID, task.DependsOn) {
		return fmt.Errorf("%w: task %s", ErrTaskDependenciesOpen, task.ID)
	}
	if err := s.ensureWorkerFreeLocked(meta.ID, botID); err != nil {
		return err
	}
	return nil
}

func (s *Service) maybeCompleteParentIfAllChildrenDoneLocked(meta TeamMeta, parentID string, actorID string, now time.Time) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return
	}

	children := s.taskChildrenLocked(meta.ID, parentID)
	if len(children) == 0 {
		return
	}
	for _, child := range children {
		if child.Status != TaskStatusCompleted {
			return
		}
	}
	parent, ok := s.tasksForTeamLocked(meta.ID)[parentID]
	if !ok || parent == nil {
		return
	}
	if parent.Status == TaskStatusCompleted {
		return
	}
	parent.Status = TaskStatusCompleted
	parent.Result = s.aggregateChildResultsLocked(meta.ID, parent.ID)
	parent.Error = ""
	parent.UpdatedAt = now
	parent.CompletedAt = timePtr(now)
	s.updatePresenceForTaskLocked(meta, parent, PresenceStateIdle, "")
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, parent),
		Type:      "task.completed",
		ActorID:   actorID,
		TaskID:    parent.ID,
		Summary:   parent.Result,
		CreatedAt: now,
	})
}

func (s *Service) bindTaskExecutionRoomLocked(meta TeamMeta, parent *TeamTask, roomID string, now time.Time) {
	if parent == nil {
		return
	}
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return
	}
	parent.RoomID = roomID
	parent.UpdatedAt = now
	for _, child := range s.taskChildrenLocked(meta.ID, parent.ID) {
		if child == nil {
			continue
		}
		child.RoomID = roomID
		child.UpdatedAt = now
	}
}

func (s *Service) readyChildrenCountLocked(meta TeamMeta, parentID string) int {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return 0
	}
	count := 0
	for _, child := range s.taskChildrenLocked(meta.ID, parentID) {
		if child == nil || child.DispatchedAt != nil {
			continue
		}
		switch child.Status {
		case TaskStatusPending, TaskStatusAssigned:
		default:
			continue
		}
		if cleanParticipantID(child.AssignedTo) == "" {
			continue
		}
		if !s.dependenciesCompletedLocked(meta.ID, child.DependsOn) {
			continue
		}
		count++
	}
	return count
}

func (s *Service) dispatchReadyChildrenLocked(meta TeamMeta, parentID string, actorID string, now time.Time) int {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return 0
	}
	children := s.taskChildrenLocked(meta.ID, parentID)
	dispatched := 0
	for _, child := range children {
		if child == nil || child.DispatchedAt != nil {
			continue
		}
		switch child.Status {
		case TaskStatusPending, TaskStatusAssigned:
		default:
			continue
		}
		assignee := cleanParticipantID(child.AssignedTo)
		if assignee == "" {
			continue
		}
		child.AssignedTo = assignee
		if !s.dependenciesCompletedLocked(meta.ID, child.DependsOn) {
			continue
		}
		child.Status = TaskStatusAssigned
		child.ClaimedBy = ""
		child.Result = ""
		child.Error = ""
		child.CompletedAt = nil
		child.DispatchedAt = timePtr(now)
		child.UpdatedAt = now
		s.appendEventLocked(meta.ID, TeamEvent{
			RoomID:    EventRoomID(meta, child),
			Type:      "task.dispatched",
			ActorID:   firstNonEmpty(strings.TrimSpace(actorID), meta.LeadBotID),
			TaskID:    child.ID,
			TargetID:  assignee,
			Summary:   child.Title,
			CreatedAt: now,
		})
		dispatched++
	}
	return dispatched
}

func (s *Service) aggregateChildResultsLocked(teamID string, parentID string) string {
	children := s.taskChildrenLocked(teamID, parentID)
	if len(children) == 0 {
		return "All child tasks completed."
	}
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].Priority != children[j].Priority {
			return children[i].Priority > children[j].Priority
		}
		if !children[i].CreatedAt.Equal(children[j].CreatedAt) {
			return children[i].CreatedAt.Before(children[j].CreatedAt)
		}
		return children[i].ID < children[j].ID
	})
	lines := []string{"All child tasks completed:"}
	for _, child := range children {
		result := strings.TrimSpace(child.Result)
		if result == "" {
			result = "completed"
		}
		lines = append(lines, fmt.Sprintf("- %s %s: %s", child.ID, child.Title, result))
	}
	return strings.Join(lines, "\n")
}

func (s *Service) claimLocked(meta TeamMeta, task *TeamTask, botID string) {
	botID = cleanParticipantID(botID)
	now := s.now()
	task.Status = TaskStatusInProgress
	task.ClaimedBy = botID
	task.UpdatedAt = now
	s.updatePresenceLocked(meta, botID, PresenceStateBusy, task.ID, "")
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    EventRoomID(meta, task),
		Type:      "task.claimed",
		ActorID:   botID,
		TaskID:    task.ID,
		Summary:   task.Title,
		CreatedAt: now,
	})
}

func (s *Service) requireTeamLocked(teamID string) (TeamMeta, error) {
	teamID = strings.TrimSpace(teamID)
	meta, ok := s.teams[teamID]
	if !ok {
		return TeamMeta{}, fmt.Errorf("%w: %s", ErrTeamNotFound, teamID)
	}
	return meta, nil
}

func (s *Service) requireTaskLocked(teamID string, taskID string) (*TeamTask, TeamMeta, error) {
	meta, err := s.requireTeamLocked(teamID)
	if err != nil {
		return nil, TeamMeta{}, err
	}
	taskID = strings.TrimSpace(taskID)
	task, ok := s.tasksForTeamLocked(teamID)[taskID]
	if !ok {
		return nil, TeamMeta{}, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	return task, meta, nil
}

func (s *Service) requireApprovalLocked(teamID string, approvalID string) (*TeamApproval, TeamMeta, error) {
	meta, err := s.requireTeamLocked(teamID)
	if err != nil {
		return nil, TeamMeta{}, err
	}
	approvalID = strings.TrimSpace(approvalID)
	approval, ok := s.approvalsForTeamLocked(teamID)[approvalID]
	if !ok {
		return nil, TeamMeta{}, fmt.Errorf("%w: %s", ErrApprovalNotFound, approvalID)
	}
	return approval, meta, nil
}

func (s *Service) requireTaskOperatorLocked(meta TeamMeta, task *TeamTask, actorID string) error {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return fmt.Errorf("actor_id is required")
	}
	if ParticipantIDsMatch(actorID, meta.LeadBotID) || ParticipantIDsMatch(actorID, task.ClaimedBy) {
		return nil
	}
	return fmt.Errorf("actor %q cannot operate task %s", actorID, task.ID)
}

func (s *Service) validateDependsOnLocked(teamID string, dependsOn []string) error {
	seen := make(map[string]struct{}, len(dependsOn))
	for _, dep := range dependsOn {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			return fmt.Errorf("depends_on contains empty task id")
		}
		if _, dup := seen[dep]; dup {
			return fmt.Errorf("depends_on contains duplicate task id %q", dep)
		}
		seen[dep] = struct{}{}
		if _, ok := s.tasksForTeamLocked(teamID)[dep]; !ok {
			return fmt.Errorf("%w: dependency %s", ErrTaskNotFound, dep)
		}
	}
	return nil
}

func (s *Service) taskChildrenLocked(teamID string, parentID string) []*TeamTask {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil
	}
	out := make([]*TeamTask, 0)
	for _, task := range s.tasksForTeamLocked(teamID) {
		if task.ParentID == parentID {
			out = append(out, task)
		}
	}
	return out
}

func (s *Service) validateParentLocked(teamID string, parentID string, pendingTaskIDs map[string]struct{}) error {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil
	}
	if _, ok := pendingTaskIDs[parentID]; ok {
		return nil
	}
	if _, ok := s.tasksForTeamLocked(teamID)[parentID]; !ok {
		return fmt.Errorf("%w: parent %s", ErrTaskNotFound, parentID)
	}
	return nil
}

func (s *Service) dependenciesCompletedLocked(teamID string, dependsOn []string) bool {
	for _, dep := range dependsOn {
		task, ok := s.tasksForTeamLocked(teamID)[dep]
		if !ok || task.Status != TaskStatusCompleted {
			return false
		}
	}
	return true
}

func (s *Service) ensureWorkerFreeLocked(teamID string, botID string) error {
	botID = cleanParticipantID(botID)
	for _, task := range s.tasksForTeamLocked(teamID) {
		if task.Status == TaskStatusInProgress && ParticipantIDsMatch(task.ClaimedBy, botID) {
			return fmt.Errorf("%w: %s", ErrWorkerAlreadyBusy, botID)
		}
	}
	return nil
}

func (s *Service) newTaskLocked(meta TeamMeta, input CreateTaskInput) *TeamTask {
	now := s.now()
	status := TaskStatusPending
	if strings.TrimSpace(input.AssignTo) != "" {
		status = TaskStatusAssigned
	}
	parentID := strings.TrimSpace(input.ParentID)
	roomID := meta.RoomID
	if parentID != "" {
		if parent, ok := s.tasksForTeamLocked(meta.ID)[parentID]; ok && parent != nil {
			roomID = firstNonEmpty(strings.TrimSpace(parent.RoomID), roomID)
		}
	}
	return &TeamTask{
		ID:         s.nextTaskIdentifier(),
		TeamID:     meta.ID,
		RoomID:     roomID,
		ParentID:   parentID,
		Title:      strings.TrimSpace(input.Title),
		Body:       strings.TrimSpace(input.Body),
		Status:     status,
		CreatedBy:  strings.TrimSpace(input.CreatedBy),
		AssignedTo: cleanParticipantID(input.AssignTo),
		DependsOn:  cloneStrings(input.DependsOn),
		Priority:   input.Priority,
		DeadlineAt: cloneTimePtr(input.DeadlineAt),
		TimeoutAt:  cloneTimePtr(input.TimeoutAt),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func (s *Service) appendEventLocked(teamID string, event TeamEvent) {
	s.nextSeq++
	event.Seq = s.nextSeq
	event.TeamID = teamID
	s.events[teamID] = append(s.events[teamID], event)
}

type capturedTeamState struct {
	exists    bool
	meta      TeamMeta
	tasks     map[string]*TeamTask
	approvals map[string]*TeamApproval
	presence  map[string]*MemberPresence
	events    []TeamEvent
}

func (s *Service) captureTeamStateLocked(teamID string) capturedTeamState {
	meta, ok := s.teams[teamID]
	if !ok {
		return capturedTeamState{}
	}
	out := capturedTeamState{
		exists:    true,
		meta:      cloneTeamMeta(meta),
		tasks:     cloneTaskMap(s.tasks[teamID]),
		approvals: cloneApprovalMap(s.approvals[teamID]),
		presence:  clonePresenceMap(s.presence[teamID]),
		events:    cloneEvents(s.events[teamID]),
	}
	return out
}

func (s *Service) restoreTeamStateLocked(teamID string, state capturedTeamState) {
	if !state.exists {
		delete(s.teams, teamID)
		delete(s.tasks, teamID)
		delete(s.approvals, teamID)
		delete(s.presence, teamID)
		delete(s.events, teamID)
		delete(s.dirtyPresence, teamID)
		return
	}
	s.teams[teamID] = cloneTeamMeta(state.meta)
	s.tasks[teamID] = cloneTaskMap(state.tasks)
	s.approvals[teamID] = cloneApprovalMap(state.approvals)
	s.presence[teamID] = clonePresenceMap(state.presence)
	s.events[teamID] = cloneEvents(state.events)
}

func (s *Service) persistMutationLocked(teamID string, before capturedTeamState, eventStart int) error {
	if s.store == nil {
		return s.projectEventsLocked(teamID, eventStart)
	}
	if eventStart < 0 || eventStart > len(s.events[teamID]) {
		return fmt.Errorf("invalid event start %d", eventStart)
	}
	snapshot := s.snapshotTeamLocked(teamID)
	newEvents := cloneEvents(s.events[teamID][eventStart:])
	if err := s.store.Save(snapshot, newEvents); err != nil {
		s.restoreTeamStateLocked(teamID, before)
		return err
	}
	delete(s.dirtyPresence, teamID)
	return s.projectEventsLocked(teamID, eventStart)
}

func (s *Service) projectEventsLocked(teamID string, eventStart int) error {
	if s.projector == nil {
		return nil
	}
	if eventStart < 0 || eventStart > len(s.events[teamID]) {
		return fmt.Errorf("invalid event start %d", eventStart)
	}
	events := cloneEvents(s.events[teamID][eventStart:])
	if len(events) == 0 {
		return nil
	}
	meta, ok := s.teams[teamID]
	if !ok {
		return nil
	}
	if err := s.projector.Project(context.Background(), cloneTeamMeta(meta), events); err != nil {
		s.recordProjectionFailureLocked(meta, events, err)
	}
	return nil
}

func (s *Service) recordProjectionFailureLocked(meta TeamMeta, events []TeamEvent, cause error) {
	if len(events) == 0 {
		return
	}
	log.Printf("team projector failed for team=%s room=%s event_seq=%d: %v", meta.ID, meta.RoomID, events[0].Seq, cause)
	before := s.captureTeamStateLocked(meta.ID)
	eventStart := len(s.events[meta.ID])
	s.appendEventLocked(meta.ID, TeamEvent{
		RoomID:    meta.RoomID,
		Type:      "projection.failed",
		ActorID:   meta.LeadBotID,
		TaskID:    firstProjectedTaskID(events),
		TargetID:  fmt.Sprintf("%d", events[0].Seq),
		Summary:   truncateSummary(cause.Error(), 240),
		CreatedAt: s.now(),
	})
	if s.store == nil {
		return
	}
	snapshot := s.snapshotTeamLocked(meta.ID)
	newEvents := cloneEvents(s.events[meta.ID][eventStart:])
	if err := s.store.Save(snapshot, newEvents); err != nil {
		log.Printf("team projector failure audit save failed for team=%s: %v", meta.ID, err)
		s.restoreTeamStateLocked(meta.ID, before)
	}
}

func firstProjectedTaskID(events []TeamEvent) string {
	for _, event := range events {
		if strings.TrimSpace(event.TaskID) != "" {
			return event.TaskID
		}
	}
	return ""
}

func truncateSummary(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit || limit <= 3 {
		return text
	}
	return text[:limit-3] + "..."
}

func (s *Service) snapshotTeamLocked(teamID string) teamSnapshot {
	meta := cloneTeamMeta(s.teams[teamID])
	tasks := make([]TeamTask, 0, len(s.tasksForTeamLocked(teamID)))
	for _, task := range s.tasksForTeamLocked(teamID) {
		tasks = append(tasks, cloneTask(*task))
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	approvals := make([]TeamApproval, 0, len(s.approvalsForTeamLocked(teamID)))
	for _, approval := range s.approvalsForTeamLocked(teamID) {
		approvals = append(approvals, cloneApproval(*approval))
	}
	sort.Slice(approvals, func(i, j int) bool { return approvals[i].ID < approvals[j].ID })

	presence := make([]MemberPresence, 0, len(s.presenceForTeamLocked(teamID)))
	for _, p := range s.presenceForTeamLocked(teamID) {
		presence = append(presence, clonePresence(*p))
	}
	sort.Slice(presence, func(i, j int) bool { return presence[i].BotID < presence[j].BotID })

	return teamSnapshot{
		Meta:      meta,
		Tasks:     tasks,
		Approvals: approvals,
		Presence:  presence,
		Events:    cloneEvents(s.events[teamID]),
	}
}

func (s *Service) markPresenceDirtyLocked(teamID string, botID string) {
	botID = cleanParticipantID(botID)
	if s.dirtyPresence[teamID] == nil {
		s.dirtyPresence[teamID] = make(map[string]struct{})
	}
	s.dirtyPresence[teamID][botID] = struct{}{}
}

func (s *Service) loadStoreState() error {
	snapshots, err := s.store.Load()
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		teamID := snapshot.Meta.ID
		s.teams[teamID] = cloneTeamMeta(snapshot.Meta)
		taskMap := make(map[string]*TeamTask, len(snapshot.Tasks))
		for _, task := range snapshot.Tasks {
			taskCopy := cloneTask(task)
			taskCopy.AssignedTo = cleanParticipantID(taskCopy.AssignedTo)
			taskCopy.ClaimedBy = cleanParticipantID(taskCopy.ClaimedBy)
			taskMap[task.ID] = &taskCopy
			s.bumpTaskIdentifierLocked(task.ID)
		}
		s.tasks[teamID] = taskMap
		approvalMap := make(map[string]*TeamApproval, len(snapshot.Approvals))
		for _, approval := range snapshot.Approvals {
			approvalCopy := cloneApproval(approval)
			approvalMap[approval.ID] = &approvalCopy
			s.bumpApprovalIdentifierLocked(approval.ID)
		}
		s.approvals[teamID] = approvalMap
		presenceMap := make(map[string]*MemberPresence, len(snapshot.Presence))
		for _, p := range snapshot.Presence {
			pCopy := clonePresence(p)
			pCopy.BotID = cleanParticipantID(pCopy.BotID)
			presenceMap[pCopy.BotID] = &pCopy
		}
		s.presence[teamID] = presenceMap
		s.events[teamID] = cloneEvents(snapshot.Events)
		s.bumpTeamIdentifierLocked(teamID)
		for _, event := range snapshot.Events {
			if event.Seq > s.nextSeq {
				s.nextSeq = event.Seq
			}
		}
	}
	return s.recoverStaleTasksLocked()
}

func (s *Service) recoverStaleTasksLocked() error {
	if s.staleTaskTTL <= 0 {
		return nil
	}
	now := s.now()
	for teamID := range s.teams {
		before := s.captureTeamStateLocked(teamID)
		eventStart := len(s.events[teamID])
		changed := false
		for _, task := range s.tasksForTeamLocked(teamID) {
			if task.Status != TaskStatusInProgress || strings.TrimSpace(task.ClaimedBy) == "" {
				continue
			}
			p := s.presenceForTeamLocked(teamID)[task.ClaimedBy]
			if p != nil && !p.LastHeartbeatAt.IsZero() && now.Sub(p.LastHeartbeatAt) <= s.staleTaskTTL {
				continue
			}
			task.Status = TaskStatusBlocked
			task.Error = "worker heartbeat stale; manual reassign required"
			task.Result = ""
			task.CompletedAt = nil
			task.UpdatedAt = now
			s.updatePresenceForTaskLocked(s.teams[teamID], task, PresenceStateBlocked, task.Error)
			s.appendEventLocked(teamID, TeamEvent{
				RoomID:    s.teams[teamID].RoomID,
				Type:      "task.blocked",
				ActorID:   task.ClaimedBy,
				TaskID:    task.ID,
				Summary:   task.Error,
				CreatedAt: now,
			})
			changed = true
		}
		if !changed {
			continue
		}
		if err := s.persistMutationLocked(teamID, before, eventStart); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) updatePresenceForTaskLocked(meta TeamMeta, task *TeamTask, state string, summary string) {
	if task.ClaimedBy == "" {
		return
	}
	s.updatePresenceLocked(meta, task.ClaimedBy, state, task.ID, summary)
}

func (s *Service) updatePresenceLocked(meta TeamMeta, botID string, state string, currentTaskID string, summary string) {
	botID = cleanParticipantID(botID)
	if botID == "" {
		return
	}
	now := s.now()
	p := s.presenceForTeamLocked(meta.ID)[botID]
	if p == nil {
		p = &MemberPresence{
			TeamID: meta.ID,
			BotID:  botID,
			Role:   "worker",
		}
		s.presenceForTeamLocked(meta.ID)[botID] = p
	}
	p.State = state
	p.CurrentTaskID = currentTaskID
	p.Summary = strings.TrimSpace(summary)
	p.LastHeartbeatAt = now
	p.UpdatedAt = now
	if ParticipantIDsMatch(botID, meta.LeadBotID) {
		p.Role = "manager"
	}
	s.markPresenceDirtyLocked(meta.ID, botID)
}

func (s *Service) tasksForTeamLocked(teamID string) map[string]*TeamTask {
	m := s.tasks[teamID]
	if m == nil {
		m = make(map[string]*TeamTask)
		s.tasks[teamID] = m
	}
	return m
}

func (s *Service) approvalsForTeamLocked(teamID string) map[string]*TeamApproval {
	m := s.approvals[teamID]
	if m == nil {
		m = make(map[string]*TeamApproval)
		s.approvals[teamID] = m
	}
	return m
}

func (s *Service) presenceForTeamLocked(teamID string) map[string]*MemberPresence {
	m := s.presence[teamID]
	if m == nil {
		m = make(map[string]*MemberPresence)
		s.presence[teamID] = m
	}
	return m
}

func (s *Service) nextTeamIdentifier() string {
	s.nextTeamID++
	return fmt.Sprintf("team-%d", s.nextTeamID)
}

func (s *Service) bumpTeamIdentifierLocked(id string) {
	s.nextTeamID = maxCounterFromIdentifier(id, "team-", s.nextTeamID)
}

func (s *Service) nextTaskIdentifier() string {
	s.nextTaskID++
	return fmt.Sprintf("task-%d", s.nextTaskID)
}

func (s *Service) bumpTaskIdentifierLocked(id string) {
	s.nextTaskID = maxCounterFromIdentifier(id, "task-", s.nextTaskID)
}

func (s *Service) peekNextTaskIdentifier(offset int) string {
	return fmt.Sprintf("task-%d", s.nextTaskID+int64(offset)+1)
}

func (s *Service) nextApprovalIdentifier() string {
	s.nextApprovalID++
	return fmt.Sprintf("approval-%d", s.nextApprovalID)
}

func (s *Service) bumpApprovalIdentifierLocked(id string) {
	s.nextApprovalID = maxCounterFromIdentifier(id, "approval-", s.nextApprovalID)
}

func cloneTeamMeta(meta TeamMeta) TeamMeta {
	return meta
}

func cloneTask(task TeamTask) TeamTask {
	task.DependsOn = cloneStrings(task.DependsOn)
	task.DispatchedAt = cloneTimePtr(task.DispatchedAt)
	task.DeadlineAt = cloneTimePtr(task.DeadlineAt)
	task.TimeoutAt = cloneTimePtr(task.TimeoutAt)
	task.CompletedAt = cloneTimePtr(task.CompletedAt)
	return task
}

func cloneApproval(approval TeamApproval) TeamApproval {
	approval.ResolvedAt = cloneTimePtr(approval.ResolvedAt)
	return approval
}

func clonePresence(p MemberPresence) MemberPresence {
	return p
}

func cloneTaskMap(in map[string]*TeamTask) map[string]*TeamTask {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*TeamTask, len(in))
	for id, task := range in {
		taskCopy := cloneTask(*task)
		out[id] = &taskCopy
	}
	return out
}

func cloneApprovalMap(in map[string]*TeamApproval) map[string]*TeamApproval {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*TeamApproval, len(in))
	for id, approval := range in {
		approvalCopy := cloneApproval(*approval)
		out[id] = &approvalCopy
	}
	return out
}

func clonePresenceMap(in map[string]*MemberPresence) map[string]*MemberPresence {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]*MemberPresence, len(in))
	for id, p := range in {
		pCopy := clonePresence(*p)
		out[id] = &pCopy
	}
	return out
}

func cloneEvents(in []TeamEvent) []TeamEvent {
	if len(in) == 0 {
		return nil
	}
	return slices.Clone(in)
}

func presenceMeaningfullyChanged(before, after MemberPresence) bool {
	return before.TeamID != after.TeamID ||
		before.BotID != after.BotID ||
		before.UserID != after.UserID ||
		before.AgentID != after.AgentID ||
		before.Role != after.Role ||
		before.State != after.State ||
		before.CurrentTaskID != after.CurrentTaskID ||
		before.Summary != after.Summary
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return slices.Clone(values)
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxCounterFromIdentifier(id string, prefix string, current int64) int64 {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, prefix) {
		return current
	}
	value, err := parseCounter(id[len(prefix):])
	if err != nil {
		return current
	}
	if value > current {
		return value
	}
	return current
}

func parseCounter(raw string) (int64, error) {
	var value int64
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid counter %q", raw)
		}
		value = value*10 + int64(ch-'0')
	}
	return value, nil
}

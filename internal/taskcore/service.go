package taskcore

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrTaskNotFound          = errors.New("task not found")
	ErrApprovalNotFound      = errors.New("approval not found")
	ErrTransitionInvalid     = errors.New("task state transition is invalid")
	ErrApprovalAlreadyClosed = errors.New("approval is not pending")
)

type Service struct {
	mu             sync.Mutex
	now            func() time.Time
	store          *Store
	taskIDs        *TaskIDAllocator
	nextApprovalID int64
	nextSeq        int64
	roots          map[string]*Task
	children       map[string]map[string]*Task
	events         map[string][]TaskEvent
	approvals      map[string]map[string]*TaskApproval
	presence       map[string]map[string]*TaskPresence
}

type Option func(*Service)

type CreateRootInput struct {
	ID               string
	AssignmentType   string
	AssignmentID     string
	Title            string
	Body             string
	CreatedBy        string
	AssignedTo       string
	ExecutionChannel string
	RoomID           string
	DependsOn        []string
}

type CreateChildInput struct {
	ParentID   string
	Title      string
	Body       string
	CreatedBy  string
	AssignedTo string
	DependsOn  []string
}

type BindRoomInput struct {
	TaskID           string
	ActorID          string
	ExecutionChannel string
	RoomID           string
}

type ClaimInput struct {
	TaskID        string
	ParticipantID string
}

type CompleteInput struct {
	TaskID  string
	ActorID string
	Result  string
}

type FailInput struct {
	TaskID  string
	ActorID string
	Error   string
}

type BlockInput struct {
	TaskID  string
	ActorID string
	Reason  string
}

type RequestApprovalInput struct {
	TaskID      string
	RequestedBy string
	ApproverID  string
	Kind        string
	Summary     string
	Payload     string
}

type ResolveApprovalInput struct {
	ApprovalID string
	ApproverID string
	Status     string
	Resolution string
}

func WithStore(store *Store) Option {
	return func(s *Service) {
		s.store = store
		if store != nil && store.TaskIDAllocator() != nil {
			s.taskIDs = store.TaskIDAllocator()
		}
	}
}

func WithNowFunc(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func NewService(opts ...Option) *Service {
	s := &Service{
		now:       time.Now,
		taskIDs:   NewMemoryTaskIDAllocator(),
		roots:     make(map[string]*Task),
		children:  make(map[string]map[string]*Task),
		events:    make(map[string][]TaskEvent),
		approvals: make(map[string]map[string]*TaskApproval),
		presence:  make(map[string]map[string]*TaskPresence),
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

func (s *Service) CreateRoot(input CreateRootInput) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateAssignment(input.AssignmentType, input.AssignmentID); err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(input.Title) == "" {
		return Task{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.CreatedBy) == "" {
		return Task{}, fmt.Errorf("created_by is required")
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		var err error
		id, err = s.nextTaskIdentifier()
		if err != nil {
			return Task{}, err
		}
	}
	if _, exists := s.roots[id]; exists {
		return Task{}, fmt.Errorf("task %q already exists", id)
	}
	if err := s.bumpTaskIdentifier(id); err != nil {
		return Task{}, err
	}
	now := s.now()
	status := StatusPending
	if strings.TrimSpace(input.AssignedTo) != "" {
		status = StatusAssigned
	}
	task := &Task{
		ID:               id,
		AssignmentType:   strings.TrimSpace(input.AssignmentType),
		AssignmentID:     strings.TrimSpace(input.AssignmentID),
		Title:            strings.TrimSpace(input.Title),
		Body:             strings.TrimSpace(input.Body),
		Status:           status,
		CreatedBy:        strings.TrimSpace(input.CreatedBy),
		AssignedTo:       strings.TrimSpace(input.AssignedTo),
		ExecutionChannel: strings.TrimSpace(input.ExecutionChannel),
		RoomID:           strings.TrimSpace(input.RoomID),
		DependsOn:        cloneStrings(input.DependsOn),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	s.roots[id] = task
	s.appendEventLocked(id, TaskEvent{
		Type:      EventTaskCreated,
		ActorID:   task.CreatedBy,
		TaskID:    task.ID,
		TargetID:  task.AssignedTo,
		Summary:   task.Title,
		CreatedAt: now,
	})
	if err := s.persistRootLocked(id, 0); err != nil {
		delete(s.roots, id)
		delete(s.events, id)
		return Task{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) CreateChild(input CreateChildInput) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootID, parent, err := s.requireTaskLocked(input.ParentID)
	if err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(parent.ParentID) != "" {
		return Task{}, fmt.Errorf("%w: only root tasks can have children", ErrTransitionInvalid)
	}
	if strings.TrimSpace(input.Title) == "" {
		return Task{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.CreatedBy) == "" {
		return Task{}, fmt.Errorf("created_by is required")
	}
	childID, err := s.nextTaskIdentifier()
	if err != nil {
		return Task{}, err
	}
	eventStart := len(s.events[rootID])
	now := s.now()
	status := StatusPending
	if strings.TrimSpace(input.AssignedTo) != "" {
		status = StatusAssigned
	}
	child := &Task{
		ID:               childID,
		ParentID:         parent.ID,
		AssignmentType:   parent.AssignmentType,
		AssignmentID:     parent.AssignmentID,
		Title:            strings.TrimSpace(input.Title),
		Body:             strings.TrimSpace(input.Body),
		Status:           status,
		CreatedBy:        strings.TrimSpace(input.CreatedBy),
		AssignedTo:       strings.TrimSpace(input.AssignedTo),
		ExecutionChannel: parent.ExecutionChannel,
		RoomID:           parent.RoomID,
		DependsOn:        cloneStrings(input.DependsOn),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	s.childrenForRootLocked(rootID)[child.ID] = child
	s.appendEventLocked(rootID, TaskEvent{
		Type:      EventTaskCreated,
		ActorID:   child.CreatedBy,
		TaskID:    child.ID,
		TargetID:  child.AssignedTo,
		Summary:   child.Title,
		CreatedAt: now,
	})
	if err := s.persistRootLocked(rootID, eventStart); err != nil {
		delete(s.childrenForRootLocked(rootID), child.ID)
		return Task{}, err
	}
	return cloneTask(*child), nil
}

func (s *Service) BindRoom(input BindRoomInput) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootID, task, err := s.requireTaskLocked(input.TaskID)
	if err != nil {
		return Task{}, err
	}
	roomID := strings.TrimSpace(input.RoomID)
	if roomID == "" {
		return Task{}, fmt.Errorf("room_id is required")
	}
	eventStart := len(s.events[rootID])
	now := s.now()
	task.RoomID = roomID
	task.ExecutionChannel = strings.TrimSpace(input.ExecutionChannel)
	task.UpdatedAt = now
	if strings.TrimSpace(task.ParentID) == "" {
		for _, child := range s.childrenForRootLocked(rootID) {
			child.RoomID = roomID
			child.ExecutionChannel = task.ExecutionChannel
			child.UpdatedAt = now
		}
	}
	s.appendEventLocked(rootID, TaskEvent{
		Type:      EventTaskExecutionRoom,
		ActorID:   strings.TrimSpace(input.ActorID),
		TaskID:    task.ID,
		TargetID:  roomID,
		Summary:   task.Title,
		CreatedAt: now,
	})
	if err := s.persistRootLocked(rootID, eventStart); err != nil {
		return Task{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) Claim(input ClaimInput) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootID, task, err := s.requireTaskLocked(input.TaskID)
	if err != nil {
		return Task{}, err
	}
	participantID := strings.TrimSpace(input.ParticipantID)
	if participantID == "" {
		return Task{}, fmt.Errorf("participant_id is required")
	}
	switch task.Status {
	case StatusPending, StatusAssigned, StatusBlocked:
	default:
		return Task{}, fmt.Errorf("%w: cannot claim task in status %s", ErrTransitionInvalid, task.Status)
	}
	if task.AssignedTo != "" && task.AssignedTo != participantID {
		return Task{}, fmt.Errorf("%w: task is assigned to %s", ErrTransitionInvalid, task.AssignedTo)
	}
	eventStart := len(s.events[rootID])
	now := s.now()
	task.Status = StatusInProgress
	task.ClaimedBy = participantID
	task.Error = ""
	task.UpdatedAt = now
	s.appendEventLocked(rootID, TaskEvent{
		Type:      EventTaskClaimed,
		ActorID:   participantID,
		TaskID:    task.ID,
		Summary:   task.Title,
		CreatedAt: now,
	})
	if err := s.persistRootLocked(rootID, eventStart); err != nil {
		return Task{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) Complete(input CompleteInput) (Task, error) {
	return s.finishTask(input.TaskID, input.ActorID, StatusCompleted, strings.TrimSpace(input.Result))
}

func (s *Service) Fail(input FailInput) (Task, error) {
	return s.finishTask(input.TaskID, input.ActorID, StatusFailed, strings.TrimSpace(input.Error))
}

func (s *Service) Block(input BlockInput) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootID, task, err := s.requireTaskLocked(input.TaskID)
	if err != nil {
		return Task{}, err
	}
	if task.Status != StatusInProgress {
		return Task{}, fmt.Errorf("%w: cannot block task in status %s", ErrTransitionInvalid, task.Status)
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return Task{}, fmt.Errorf("reason is required")
	}
	eventStart := len(s.events[rootID])
	now := s.now()
	task.Status = StatusBlocked
	task.Error = reason
	task.Result = ""
	task.CompletedAt = nil
	task.UpdatedAt = now
	s.appendEventLocked(rootID, TaskEvent{
		Type:      EventTaskBlocked,
		ActorID:   strings.TrimSpace(input.ActorID),
		TaskID:    task.ID,
		Summary:   reason,
		CreatedAt: now,
	})
	if err := s.persistRootLocked(rootID, eventStart); err != nil {
		return Task{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) RequestApproval(input RequestApprovalInput) (TaskApproval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootID, task, err := s.requireTaskLocked(input.TaskID)
	if err != nil {
		return TaskApproval{}, err
	}
	if strings.TrimSpace(input.RequestedBy) == "" {
		return TaskApproval{}, fmt.Errorf("requested_by is required")
	}
	if strings.TrimSpace(input.Kind) == "" {
		return TaskApproval{}, fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(input.Summary) == "" {
		return TaskApproval{}, fmt.Errorf("summary is required")
	}
	eventStart := len(s.events[rootID])
	now := s.now()
	approval := &TaskApproval{
		ID:             s.nextApprovalIdentifier(),
		AssignmentType: task.AssignmentType,
		AssignmentID:   task.AssignmentID,
		RoomID:         task.RoomID,
		TaskID:         task.ID,
		RequestedBy:    strings.TrimSpace(input.RequestedBy),
		ApproverID:     strings.TrimSpace(input.ApproverID),
		Kind:           strings.TrimSpace(input.Kind),
		Summary:        strings.TrimSpace(input.Summary),
		Payload:        strings.TrimSpace(input.Payload),
		Status:         ApprovalStatusPending,
		CreatedAt:      now,
	}
	s.approvalsForRootLocked(rootID)[approval.ID] = approval
	s.appendEventLocked(rootID, TaskEvent{
		Type:      EventApprovalRequested,
		ActorID:   approval.RequestedBy,
		TaskID:    task.ID,
		TargetID:  approval.ID,
		Summary:   approval.Summary,
		CreatedAt: now,
	})
	if err := s.persistRootLocked(rootID, eventStart); err != nil {
		return TaskApproval{}, err
	}
	return cloneApproval(*approval), nil
}

func (s *Service) ResolveApproval(input ResolveApprovalInput) (TaskApproval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootID, approval, err := s.requireApprovalLocked(input.ApprovalID)
	if err != nil {
		return TaskApproval{}, err
	}
	if approval.Status != ApprovalStatusPending {
		return TaskApproval{}, ErrApprovalAlreadyClosed
	}
	status := strings.TrimSpace(input.Status)
	switch status {
	case ApprovalStatusApproved, ApprovalStatusRejected, ApprovalStatusCancelled:
	default:
		return TaskApproval{}, fmt.Errorf("unsupported approval status %q", input.Status)
	}
	eventStart := len(s.events[rootID])
	now := s.now()
	approval.Status = status
	approval.ApproverID = firstNonEmpty(strings.TrimSpace(input.ApproverID), approval.ApproverID)
	approval.Resolution = strings.TrimSpace(input.Resolution)
	approval.ResolvedAt = timePtr(now)
	s.appendEventLocked(rootID, TaskEvent{
		Type:      EventApprovalResolved,
		ActorID:   approval.ApproverID,
		TaskID:    approval.TaskID,
		TargetID:  approval.ID,
		Summary:   approval.Status,
		CreatedAt: now,
	})
	if err := s.persistRootLocked(rootID, eventStart); err != nil {
		return TaskApproval{}, err
	}
	return cloneApproval(*approval), nil
}

func (s *Service) Get(taskID string) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, task, err := s.requireTaskLocked(taskID)
	if err != nil {
		return Task{}, false
	}
	return cloneTask(*task), true
}

func (s *Service) ListGlobal() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Task, 0)
	for _, root := range s.roots {
		out = append(out, cloneTask(*root))
		for _, child := range s.childrenForRootLocked(root.ID) {
			out = append(out, cloneTask(*child))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Service) ListByAssignment(assignmentType, assignmentID string) []Task {
	assignmentType = strings.TrimSpace(assignmentType)
	assignmentID = strings.TrimSpace(assignmentID)
	tasks := s.ListGlobal()
	out := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.AssignmentType == assignmentType && task.AssignmentID == assignmentID {
			out = append(out, task)
		}
	}
	return out
}

func (s *Service) Events(rootID string) []TaskEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneEvents(s.events[strings.TrimSpace(rootID)])
}

func (s *Service) finishTask(taskID, actorID, status, summary string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootID, task, err := s.requireTaskLocked(taskID)
	if err != nil {
		return Task{}, err
	}
	if task.Status != StatusInProgress {
		return Task{}, fmt.Errorf("%w: cannot finish task in status %s", ErrTransitionInvalid, task.Status)
	}
	if summary == "" {
		if status == StatusCompleted {
			return Task{}, fmt.Errorf("result is required")
		}
		return Task{}, fmt.Errorf("error is required")
	}
	eventStart := len(s.events[rootID])
	now := s.now()
	task.Status = status
	task.UpdatedAt = now
	if status == StatusCompleted {
		task.Result = summary
		task.Error = ""
		task.CompletedAt = timePtr(now)
	} else {
		task.Error = summary
		task.Result = ""
		task.CompletedAt = nil
	}
	eventType := EventTaskCompleted
	if status == StatusFailed {
		eventType = EventTaskFailed
	}
	s.appendEventLocked(rootID, TaskEvent{
		Type:      eventType,
		ActorID:   strings.TrimSpace(actorID),
		TaskID:    task.ID,
		Summary:   summary,
		CreatedAt: now,
	})
	if err := s.persistRootLocked(rootID, eventStart); err != nil {
		return Task{}, err
	}
	return cloneTask(*task), nil
}

func (s *Service) requireTaskLocked(taskID string) (string, *Task, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", nil, fmt.Errorf("%w: empty id", ErrTaskNotFound)
	}
	if root, ok := s.roots[taskID]; ok {
		return taskID, root, nil
	}
	for rootID, children := range s.children {
		if task, ok := children[taskID]; ok {
			return rootID, task, nil
		}
	}
	return "", nil, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
}

func (s *Service) requireApprovalLocked(approvalID string) (string, *TaskApproval, error) {
	approvalID = strings.TrimSpace(approvalID)
	for rootID, approvals := range s.approvals {
		if approval, ok := approvals[approvalID]; ok {
			return rootID, approval, nil
		}
	}
	return "", nil, fmt.Errorf("%w: %s", ErrApprovalNotFound, approvalID)
}

func (s *Service) appendEventLocked(rootID string, event TaskEvent) {
	root := s.roots[rootID]
	if root == nil {
		return
	}
	s.nextSeq++
	event.Seq = s.nextSeq
	event.AssignmentType = root.AssignmentType
	event.AssignmentID = root.AssignmentID
	if strings.TrimSpace(event.RoomID) == "" {
		if _, task, err := s.requireTaskLocked(event.TaskID); err == nil {
			event.RoomID = task.RoomID
			event.Channel = task.ExecutionChannel
		}
	}
	s.events[rootID] = append(s.events[rootID], event)
}

func (s *Service) persistRootLocked(rootID string, eventStart int) error {
	if s.store == nil {
		return nil
	}
	if eventStart < 0 || eventStart > len(s.events[rootID]) {
		return fmt.Errorf("invalid event start %d", eventStart)
	}
	return s.store.SaveSnapshot(s.snapshotLocked(rootID), cloneEvents(s.events[rootID][eventStart:]))
}

func (s *Service) snapshotLocked(rootID string) Snapshot {
	children := make([]Task, 0, len(s.childrenForRootLocked(rootID)))
	for _, child := range s.childrenForRootLocked(rootID) {
		children = append(children, cloneTask(*child))
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })

	approvals := make([]TaskApproval, 0, len(s.approvalsForRootLocked(rootID)))
	for _, approval := range s.approvalsForRootLocked(rootID) {
		approvals = append(approvals, cloneApproval(*approval))
	}
	sort.Slice(approvals, func(i, j int) bool { return approvals[i].ID < approvals[j].ID })

	presence := make([]TaskPresence, 0, len(s.presenceForRootLocked(rootID)))
	for _, p := range s.presenceForRootLocked(rootID) {
		presence = append(presence, clonePresence(*p))
	}
	sort.Slice(presence, func(i, j int) bool { return presence[i].ParticipantID < presence[j].ParticipantID })

	return Snapshot{
		Root:      cloneTask(*s.roots[rootID]),
		Children:  children,
		Approvals: approvals,
		Presence:  presence,
		Events:    cloneEvents(s.events[rootID]),
	}
}

func (s *Service) loadStoreState() error {
	snapshots, err := s.store.Load()
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		root := cloneTask(snapshot.Root)
		s.roots[root.ID] = &root
		if err := s.bumpTaskIdentifier(root.ID); err != nil {
			return err
		}
		childMap := make(map[string]*Task, len(snapshot.Children))
		for _, child := range snapshot.Children {
			childCopy := cloneTask(child)
			childMap[childCopy.ID] = &childCopy
			if err := s.bumpTaskIdentifier(childCopy.ID); err != nil {
				return err
			}
		}
		s.children[root.ID] = childMap
		approvalMap := make(map[string]*TaskApproval, len(snapshot.Approvals))
		for _, approval := range snapshot.Approvals {
			approvalCopy := cloneApproval(approval)
			approvalMap[approvalCopy.ID] = &approvalCopy
			s.bumpApprovalIdentifier(approvalCopy.ID)
		}
		s.approvals[root.ID] = approvalMap
		presenceMap := make(map[string]*TaskPresence, len(snapshot.Presence))
		for _, p := range snapshot.Presence {
			pCopy := clonePresence(p)
			presenceMap[pCopy.ParticipantID] = &pCopy
		}
		s.presence[root.ID] = presenceMap
		s.events[root.ID] = cloneEvents(snapshot.Events)
		for _, event := range snapshot.Events {
			if event.Seq > s.nextSeq {
				s.nextSeq = event.Seq
			}
		}
	}
	return nil
}

func (s *Service) childrenForRootLocked(rootID string) map[string]*Task {
	m := s.children[rootID]
	if m == nil {
		m = make(map[string]*Task)
		s.children[rootID] = m
	}
	return m
}

func (s *Service) approvalsForRootLocked(rootID string) map[string]*TaskApproval {
	m := s.approvals[rootID]
	if m == nil {
		m = make(map[string]*TaskApproval)
		s.approvals[rootID] = m
	}
	return m
}

func (s *Service) presenceForRootLocked(rootID string) map[string]*TaskPresence {
	m := s.presence[rootID]
	if m == nil {
		m = make(map[string]*TaskPresence)
		s.presence[rootID] = m
	}
	return m
}

func (s *Service) nextTaskIdentifier() (string, error) {
	if s.taskIDs == nil {
		s.taskIDs = NewMemoryTaskIDAllocator()
	}
	return s.taskIDs.Next()
}

func (s *Service) nextApprovalIdentifier() string {
	s.nextApprovalID++
	return fmt.Sprintf("approval-%d", s.nextApprovalID)
}

func (s *Service) bumpTaskIdentifier(id string) error {
	if s.taskIDs == nil {
		s.taskIDs = NewMemoryTaskIDAllocator()
	}
	return s.taskIDs.Bump(id)
}

func (s *Service) bumpApprovalIdentifier(id string) {
	s.nextApprovalID = maxCounterFromIdentifier(id, "approval-", s.nextApprovalID)
}

func validateAssignment(assignmentType, assignmentID string) error {
	switch strings.TrimSpace(assignmentType) {
	case AssignmentTypeTeam, AssignmentTypeAgent:
	default:
		return fmt.Errorf("unsupported assignment_type %q", assignmentType)
	}
	if strings.TrimSpace(assignmentID) == "" {
		return fmt.Errorf("assignment_id is required")
	}
	return nil
}

func cloneTask(task Task) Task {
	task.DependsOn = cloneStrings(task.DependsOn)
	task.DispatchedAt = cloneTimePtr(task.DispatchedAt)
	task.DeadlineAt = cloneTimePtr(task.DeadlineAt)
	task.TimeoutAt = cloneTimePtr(task.TimeoutAt)
	task.CompletedAt = cloneTimePtr(task.CompletedAt)
	return task
}

func cloneApproval(approval TaskApproval) TaskApproval {
	approval.ResolvedAt = cloneTimePtr(approval.ResolvedAt)
	return approval
}

func clonePresence(p TaskPresence) TaskPresence {
	return p
}

func cloneEvents(in []TaskEvent) []TaskEvent {
	if len(in) == 0 {
		return nil
	}
	out := make([]TaskEvent, len(in))
	copy(out, in)
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
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
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maxCounterFromIdentifier(id string, prefix string, current int64) int64 {
	id = strings.TrimSpace(id)
	if !strings.HasPrefix(id, prefix) {
		return current
	}
	var value int64
	for _, ch := range id[len(prefix):] {
		if ch < '0' || ch > '9' {
			return current
		}
		value = value*10 + int64(ch-'0')
	}
	if value > current {
		return value
	}
	return current
}

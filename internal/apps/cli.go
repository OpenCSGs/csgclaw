package apps

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"csgclaw/internal/connectorcli"
)

func (s *Service) isCLI(agentID, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked(agentID, id)
	return err == nil && e.record.Kind == KindCLI
}

func (s *Service) EnsureFeishuCLI(ctx context.Context) (Installation, error) {
	return s.ensureCLI(ctx, "feishu")
}

func (s *Service) ensureCLI(_ context.Context, appID string) (Installation, error) {
	pkg, ok := s.packages[appID]
	if !ok || pkg.CLI == nil {
		return Installation{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.record.AgentID == "" && e.record.Kind == KindCLI && e.record.AppID == appID {
			return s.viewLocked(e), nil
		}
	}
	id, err := newInstallationID()
	if err != nil {
		return Installation{}, err
	}
	now := time.Now().UTC()
	r := record{Installation: Installation{Kind: KindCLI, InstallationID: id, AppID: appID, Name: pkg.Manifest.Interface.DisplayName, Enabled: true, ResourceEnabled: true, Revision: 1, Status: "needs_configuration", CreatedAt: now, UpdatedAt: now}, Package: pkg, CLI: &cliRecord{ConfigDir: filepath.Join(s.dataRoot, id, "config"), Completed: map[string]CLIOperation{}}}
	if err = s.persistLocked(id, &r); err != nil {
		return Installation{}, err
	}
	e := &entry{record: r}
	s.entries[id] = e
	return s.viewLocked(e), nil
}

func (s *Service) bindCLI(ctx context.Context, agentID, id string) (Installation, error) {
	if strings.TrimSpace(agentID) == "" {
		return Installation{}, ErrInvalid
	}
	if s.options.Runtime == nil {
		return Installation{}, ErrInvalid
	}
	if _, err := s.options.Runtime.State(ctx, agentID, KindCLI); err != nil {
		return Installation{}, err
	}
	s.mu.Lock()
	resource, err := s.findLocked("", id)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	for _, e := range s.entries {
		if e.record.AgentID == agentID && e.record.ResourceID == id {
			bid := e.record.InstallationID
			s.mu.Unlock()
			return s.SetCLIEnabled(ctx, agentID, bid, true)
		}
	}
	bid, err := newInstallationID()
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	now := time.Now().UTC()
	r := record{Installation: Installation{InstallationID: bid, AgentID: agentID, ResourceID: id, Enabled: true, Status: "applying", CreatedAt: now, UpdatedAt: now}, CLIApply: &cliBinding{Pending: true}}
	hydrateBinding(&r, resource.record)
	if err = s.persistLocked(bid, &r); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	e := &entry{record: r}
	s.entries[bid] = e
	out := s.viewLocked(e)
	s.mu.Unlock()
	s.scheduleConnectorWork()
	return out, nil
}
func (s *Service) SetCLIEnabled(_ context.Context, agentID, id string, enabled bool) (Installation, error) {
	s.mu.Lock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	if agentID == "" || e.record.Kind != KindCLI {
		s.mu.Unlock()
		return Installation{}, ErrInvalid
	}
	next := copyJSON(e.record)
	if next.CLIApply != nil && next.CLIApply.Pending && next.Enabled != enabled && next.LastErrorCode == "" {
		s.mu.Unlock()
		return Installation{}, ErrConflict
	}
	if next.Enabled == enabled && next.CLIApply != nil && !next.CLIApply.Pending && next.LastErrorCode == "" {
		out := s.viewLocked(e)
		s.mu.Unlock()
		return out, nil
	}
	next.Enabled = enabled
	next.Status = "applying"
	next.LastErrorCode = ""
	next.LastError = ""
	if next.CLIApply == nil {
		next.CLIApply = &cliBinding{}
	}
	next.CLIApply.Pending = true
	next.CLIApply.Remove = false
	next.UpdatedAt = time.Now().UTC()
	if err = s.persistLocked(id, &next); err != nil {
		s.mu.Unlock()
		return Installation{}, err
	}
	e.record = next
	if m := s.cliMigration; m != nil && m.Status == "waiting_for_legacy_callers" && m.Workers[agentID] != nil {
		migration := copyJSON(m)
		migration.Status = "pending"
		if err := s.saveMigrationLocked(migration); err != nil {
			s.mu.Unlock()
			return Installation{}, err
		}
		s.cliMigration = migration
	}
	out := s.viewLocked(e)
	s.mu.Unlock()
	s.scheduleConnectorWork()
	return out, nil
}
func (s *Service) deleteCLIBinding(ctx context.Context, agentID, id string) error {
	if agentID == "" {
		return fmt.Errorf("%w: disconnect the global CLI connector", ErrInvalid)
	}
	if _, err := s.SetCLIEnabled(ctx, agentID, id, false); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked(agentID, id)
	if err != nil {
		return err
	}
	next := copyJSON(e.record)
	next.CLIApply.Pending = true
	next.CLIApply.Remove = true
	if err := s.persistLocked(id, &next); err != nil {
		return err
	}
	e.record = next
	s.scheduleConnectorWork()
	return nil
}

func activeCLI(op *cliOperation) bool {
	return op != nil && (op.Status == "running" || op.Status == "pending")
}

// StartCLI records intent before the background task acquires any Agent lifecycle lock.
func (s *Service) StartCLI(_ context.Context, id string, in CLIRequest) (CLIOperation, error) {
	switch in.Action {
	case "connect", "login", "logout", "disconnect", "check":
	default:
		return CLIOperation{}, ErrInvalid
	}
	if in.OperationID == "" || len(in.OperationID) > 128 {
		return CLIOperation{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked("", id)
	if err != nil {
		return CLIOperation{}, err
	}
	if e.record.CLI == nil {
		return CLIOperation{}, ErrInvalid
	}
	state := e.record.CLI
	if op := state.Queued; op != nil {
		if op.ID == in.OperationID && op.Action == in.Action && op.Revision == in.Revision {
			return op.CLIOperation, nil
		}
		return CLIOperation{}, ErrConflict
	}
	if old, ok := state.Completed[in.OperationID]; ok {
		if old.Action != in.Action || old.Revision != in.Revision {
			return CLIOperation{}, ErrConflict
		}
		return old, nil
	}
	if op := state.Operation; op != nil && op.ID == in.OperationID {
		if op.Action != in.Action || op.Revision != in.Revision {
			return CLIOperation{}, ErrConflict
		}
		return op.CLIOperation, nil
	}
	if in.Revision != e.record.Revision {
		return CLIOperation{}, ErrChanged
	}
	if m := s.cliMigration; m != nil && m.ResourceID == id && m.Status == "disconnecting" && in.Action != "disconnect" {
		return CLIOperation{}, ErrConflict
	}
	if activeCLI(state.Operation) {
		if state.Operation.Action == in.Action {
			return state.Operation.CLIOperation, nil
		}
		if in.Action != "disconnect" {
			return CLIOperation{}, ErrConflict
		}
		session, err := newInstallationID()
		if err != nil {
			return CLIOperation{}, err
		}
		next := copyJSON(e.record)
		next.CLI.Queued = &cliOperation{CLIOperation: CLIOperation{ID: in.OperationID, SessionID: session, ResourceID: id, Revision: in.Revision, Action: in.Action, Stage: "pending", Status: "pending", UpdatedAt: time.Now().UTC()}}
		next.CLI.Operation.Cancelled = true
		if err := s.persistLocked(id, &next); err != nil {
			return CLIOperation{}, err
		}
		e.record = next
		if s.cliCancel != nil && s.cliCancelSession == state.Operation.SessionID {
			s.cliCancel()
		}
		s.scheduleConnectorWork()
		return next.CLI.Queued.CLIOperation, nil
	}
	session, err := newInstallationID()
	if err != nil {
		return CLIOperation{}, err
	}
	op := &cliOperation{CLIOperation: CLIOperation{ID: in.OperationID, SessionID: session, ResourceID: id, Revision: in.Revision, Action: in.Action, Stage: "pending", Status: "pending", UpdatedAt: time.Now().UTC()}, Workers: map[string]bool{}}
	next := copyJSON(e.record)
	// Failed tasks retain original running intent until a subsequent operation succeeds.
	if old := next.CLI.Operation; old != nil && !old.Settled {
		for worker, running := range old.Workers {
			op.Workers[worker] = running
		}
	}
	if old := next.CLI.Operation; old != nil && !old.Settled && next.CLI.Recovery == nil {
		next.CLI.Recovery = copyJSON(old)
	}
	next.CLI.Operation = op
	if err = s.persistLocked(id, &next); err != nil {
		return CLIOperation{}, err
	}
	e.record = next
	if in.Action == "connect" && s.cliMigration != nil && s.cliMigration.ResourceID == id && s.cliMigration.Status != "completed" {
		migration := copyJSON(s.cliMigration)
		migration.Status = "pending"
		if err := s.saveMigrationLocked(migration); err != nil {
			return CLIOperation{}, err
		}
		s.cliMigration = migration
	}
	s.scheduleConnectorWork()
	return op.CLIOperation, nil
}
func (s *Service) CLISession(_ context.Context, id, session string) (CLIOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked("", id)
	if err != nil {
		return CLIOperation{}, err
	}
	if e.record.CLI == nil {
		return CLIOperation{}, ErrInvalid
	}
	if op := e.record.CLI.Queued; op != nil && op.SessionID == session {
		return op.CLIOperation, nil
	}
	if op := e.record.CLI.Operation; op != nil && op.SessionID == session {
		return op.CLIOperation, nil
	}
	for _, op := range e.record.CLI.Completed {
		if op.SessionID == session {
			return op, nil
		}
	}
	return CLIOperation{}, ErrNotFound
}
func (s *Service) CancelCLI(_ context.Context, id, session string) (CLIOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked("", id)
	if err != nil {
		return CLIOperation{}, err
	}
	if e.record.CLI == nil || e.record.CLI.Operation == nil || e.record.CLI.Operation.SessionID != session {
		return CLIOperation{}, ErrNotFound
	}
	op := e.record.CLI.Operation
	if !activeCLI(op) {
		return op.CLIOperation, nil
	}
	next := copyJSON(e.record)
	next.CLI.Operation.Cancelled = true
	if err = s.persistLocked(id, &next); err != nil {
		return CLIOperation{}, err
	}
	e.record = next
	if s.cliCancel != nil && s.cliCancelSession == op.SessionID {
		s.cliCancel()
	}
	s.scheduleConnectorWork()
	return next.CLI.Operation.CLIOperation, nil
}

func (s *Service) CheckCLIReady(agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.cliMigration; m != nil && m.Status != "completed" && m.Workers[agentID] != nil {
		if m.Status == "waiting_for_legacy_callers" && m.Workers[agentID].Phase == "original" && m.Workers[agentID].Recovered {
			return nil
		}
		return errors.New("Feishu CLI migration is pending")
	}
	for _, e := range s.entries {
		if e.record.AgentID != agentID || (e.record.Kind != KindCLI && e.record.CLIApply == nil) || !bindingNeedsConfiguration(e.record) {
			continue
		}
		if e.record.CLIApply != nil && e.record.CLIApply.Pending {
			return errors.New("Connector configuration is pending")
		}
		resource := s.entries[e.record.ResourceID]
		if resource != nil && resource.record.CLI != nil {
			op := resource.record.CLI.Operation
			if op != nil && !op.Settled {
				return errors.New("CLI connector operation is pending")
			}
		}
	}
	return nil
}
func (s *Service) CLIProjection(agentID, resourceID string) (*CLIProjection, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.record.AgentID != agentID || e.record.ResourceID != resourceID || e.record.Kind != KindCLI || !e.record.Enabled {
			continue
		}
		resource := s.entries[e.record.ResourceID]
		if resource == nil || resource.record.CLI == nil {
			return nil, 0, ErrNotFound
		}
		r := resource.record
		if r.Disconnected || !cliReady(r) {
			return nil, r.Revision, nil
		}
		return cliPayload(agentID, r), r.Revision, nil
	}
	return nil, 0, nil
}
func (s *Service) scheduleConnectorWork() {
	select {
	case s.cliWake <- struct{}{}:
	default:
	}
}
func (s *Service) runConnectorWork() {
	defer s.cliWG.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		var done chan struct{}
		select {
		case done = <-s.cliDrain:
		case <-s.ctx.Done():
			return
		case <-s.cliWake:
		case <-ticker.C:
			s.cliWorkMu.Lock()
			_ = s.cleanupMigration(s.ctx)
			s.cliWorkMu.Unlock()
			continue
		}
		for {
			s.processConnectorWork()
			if done == nil {
				break
			}
			select {
			case <-s.cliWake:
				continue
			default:
				close(done)
			}
			break
		}
	}
}

func (s *Service) processConnectorWork() {
	s.cliWorkMu.Lock()
	s.mu.Lock()
	var queued []string
	for id, e := range s.entries {
		if e.record.CLI != nil && e.record.CLI.Queued != nil {
			queued = append(queued, id)
		}
	}
	s.mu.Unlock()
	for _, id := range queued {
		s.promoteQueuedCLI(id)
	}
	if m := s.migrationSnapshot(); m != nil && (m.Status == "pending" || m.Status == "running") && !s.migrationDisconnectRequested() {
		_ = s.executeMigration(s.ctx)
	}
	s.mu.Lock()
	var operations, bindings []string
	for id, e := range s.entries {
		if e.record.CLI != nil && activeCLI(e.record.CLI.Operation) {
			operations = append(operations, id)
		}
		if e.record.CLIApply != nil && e.record.CLIApply.Pending && e.record.LastErrorCode == "" && s.cliResourceSettledLocked(e.record.ResourceID) {
			bindings = append(bindings, id)
		}
	}
	s.mu.Unlock()
	sort.Strings(operations)
	sort.Strings(bindings)
	for _, id := range operations {
		s.executeCLI(id)
	}
	for _, id := range bindings {
		if s.ctx.Err() == nil {
			_ = s.applyBinding(s.ctx, id)
		}
	}
	s.cliWorkMu.Unlock()
}

func (s *Service) editCLI(id string, fn func(*record) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked("", id)
	if err != nil {
		return err
	}
	next := copyJSON(e.record)
	if err = fn(&next); err != nil {
		return err
	}
	next.UpdatedAt = time.Now().UTC()
	if next.CLI.Operation != nil {
		next.CLI.Operation.UpdatedAt = next.UpdatedAt
	}
	if err = s.persistLocked(id, &next); err != nil {
		return err
	}
	e.record = next
	for _, b := range s.entries {
		if b.record.ResourceID == id {
			hydrateBinding(&b.record, next)
		}
	}
	return nil
}
func (s *Service) cliSnapshot(id string) (record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.findLocked("", id)
	if err != nil {
		return record{}, err
	}
	return copyJSON(e.record), nil
}

func setCLIOperationError(r *record) {
	r.LastErrorCode = r.CLI.Operation.ErrorCode
	r.LastErrorHTTPStatus = 0
	r.LastError = ""
	if r.LastErrorCode != "" {
		r.LastError = "CLI connector operation could not be completed; retry the operation."
	}
}

func repairCancelledCLIError(r *record) bool {
	if r.Kind != KindCLI || r.AgentID != "" || r.CLI == nil || r.CLI.Operation == nil {
		return false
	}
	op := r.CLI.Operation
	if op.Status != "cancelled" || !op.Settled || op.ErrorCode != "" || r.LastErrorCode != "" {
		return false
	}
	if r.LastError == "" && r.LastErrorHTTPStatus == 0 {
		return false
	}
	// Older versions saved a failure message even after cancellation recovered successfully.
	setCLIOperationError(r)
	return true
}

func (s *Service) executeCLI(id string) {
	defer s.promoteQueuedCLI(id)
	r, err := s.cliSnapshot(id)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.mu.Lock()
	s.cliCancel = cancel
	s.cliCancelSession = r.CLI.Operation.SessionID
	if s.entries[id].record.CLI.Operation.Cancelled {
		cancel()
	}
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.cliCancel = nil
		s.cliCancelSession = ""
		s.mu.Unlock()
	}()
	err = s.performCLI(ctx, id)
	if s.ctx.Err() != nil {
		return
	} // Saved stage and process identity are recovered on startup.
	if err != nil {
		latest, _ := s.cliSnapshot(id)
		cancelled := ctx.Err() != nil
		settled := false
		migration := s.migrationSnapshot()
		migrationReady := migration == nil || migration.ResourceID != id || migration.Status == "completed"
		if migration != nil && migration.Status == "disconnecting" && latest.CLI != nil && latest.CLI.Operation.Action == "disconnect" && latest.CLI.Operation.Committed {
			migrationReady = true
		}
		if migrationReady && latest.CLI != nil && latest.CLI.Recovery == nil && (!latest.CLI.Operation.Committed || cancelled) {
			recovery, stop := context.WithTimeout(s.ctx, 30*time.Second)
			var restoreErr error
			if !latest.CLI.Operation.Committed {
				restoreErr = s.recoverCLIStatus(recovery, id)
			}
			// Committed authorization remains authoritative while cancellation finishes worker updates.
			if restoreErr == nil {
				restoreErr = s.finishCLIWorkers(recovery, id)
			}
			stop()
			settled = restoreErr == nil
		}
		_ = s.editCLI(id, func(r *record) error {
			op := r.CLI.Operation
			op.Settled = settled
			op.Status = "failed"
			op.FailureStage = op.Stage
			op.ErrorCode = "cli_operation_failed"
			var commandErr *connectorcli.OperationError
			if errors.As(err, &commandErr) {
				op.ErrorCode = commandErr.Code
				if settled && commandErr.Code == "authorization_expired" {
					op.Status = "expired"
				}
				if settled && commandErr.Code == "authorization_denied" {
					op.Status = "denied"
				}
			}
			if cancelled && settled {
				op.Status = "cancelled"
				op.ErrorCode = ""
			}
			if !settled {
				op.ErrorCode = "cleanup_pending"
			}
			op.URL = ""
			op.Device = nil
			setCLIOperationError(r)
			if r.CLI.Completed == nil {
				r.CLI.Completed = map[string]CLIOperation{}
			}
			r.CLI.Completed[op.ID] = op.CLIOperation
			return nil
		})
		return
	}

	_ = s.editCLI(id, func(r *record) error {
		op := r.CLI.Operation
		op.Settled = true
		op.Status = "completed"
		op.Stage = "completed"
		op.URL = ""
		op.Device = nil
		op.ErrorCode = ""
		setCLIOperationError(r)
		if r.CLI.Completed == nil {
			r.CLI.Completed = map[string]CLIOperation{}
		}
		r.CLI.Completed[op.ID] = op.CLIOperation
		return nil
	})
}
func (s *Service) performCLI(ctx context.Context, id string) error {
	r, err := s.cliSnapshot(id)
	if err != nil {
		return err
	}
	if m := s.migrationSnapshot(); m != nil && m.ResourceID == id && m.Status != "completed" && r.CLI.Operation.Action != "disconnect" {
		return errors.New("waiting_for_legacy_callers")
	}
	op := r.CLI.Operation
	if err = connectorcli.RecoverProcess(op.Process); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = s.editCLI(id, func(r *record) error {
		r.CLI.Operation.Status = "running"
		r.CLI.Operation.Process = connectorcli.ProcessRef{}
		return nil
	}); err != nil {
		return err
	}
	if op.Action == "disconnect" {
		if err = s.editCLI(id, func(r *record) error { r.CLI.Operation.Stage = "stopping_workers"; return nil }); err != nil {
			return err
		}
		if err = s.prepareMigrationDisconnect(ctx, id); err != nil {
			return err
		}
	}
	if err = s.suspendCLIWorkers(ctx, id); err != nil {
		return err
	}
	if r.CLI.Recovery != nil {
		if err = connectorcli.RecoverProcess(r.CLI.Recovery.Process); err != nil {
			return err
		}
		if err = s.editCLI(id, func(r *record) error { r.CLI.Recovery = nil; return nil }); err != nil {
			return err
		}
	}
	if op.Committed {
		return s.finishCLIWorkers(ctx, id)
	}
	if op.Action != "disconnect" {
		pkg, err := s.installCLI(ctx, filepath.Join(s.dataRoot, id, "package"), r.Package.CLI.Install, func(stage string) {
			_ = s.editCLI(id, func(r *record) error { r.CLI.Operation.Stage = stage; return nil })
		})
		if err != nil {
			return err
		}
		if err = s.editCLI(id, func(r *record) error {
			r.CLI.Executable = pkg.Executable
			r.CLI.SkillsDir = pkg.SkillsDir
			r.PackageVersion = pkg.Version
			r.SkillVersion = r.Package.CLI.Install.Skills.Version
			return nil
		}); err != nil {
			return err
		}
	}
	r, err = s.cliSnapshot(id)
	if err != nil {
		return err
	}
	if len(r.Package.SkillFiles) > 0 && op.Action != "disconnect" {
		bundle, err := s.packageSkills(r)
		if err != nil {
			return err
		}
		if err = s.editCLI(id, func(r *record) error {
			r.CLI.SkillsDir = bundle
			r.SkillVersion = r.Package.Manifest.Version
			return nil
		}); err != nil {
			return err
		}
	}
	if err = s.editCLI(id, func(r *record) error { r.CLI.Operation.CommandsStarted = true; return nil }); err != nil {
		return err
	}
	ready, status, err := s.authorizeCLI(ctx, id, r)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = s.editCLI(id, func(r *record) error {
		if r.CLI.Operation.Cancelled {
			return context.Canceled
		}
		r.AuthStatus = status
		r.RequestedScopes = r.Package.CLI.RequestedScopes
		r.CLI.Ready = ready
		r.CheckedAt = time.Now().UTC()
		r.Disconnected = op.Action == "disconnect"
		if r.Disconnected {
			r.RequestedScopes = ""
		}
		r.Revision++
		r.CLI.Operation.Committed = true
		r.CLI.Operation.Stage = "applying"
		r.CLI.Operation.URL = ""
		r.CLI.Operation.Device = nil
		return nil
	}); err != nil {
		return err
	}
	return s.finishCLIWorkers(ctx, id)
}

func (s *Service) finishCLIWorkers(ctx context.Context, id string) error {
	if err := s.applyCLIWorkers(ctx, id); err != nil {
		return err
	}
	return s.finishMigrationDisconnect(id)
}

func (s *Service) suspendCLIWorkers(ctx context.Context, id string) error {
	s.mu.Lock()
	var workers []string
	for _, e := range s.entries {
		if e.record.ResourceID == id && bindingNeedsConfiguration(e.record) {
			workers = append(workers, e.record.AgentID)
		}
	}
	s.mu.Unlock()
	sort.Strings(workers)
	if len(workers) == 0 {
		return nil
	}
	if s.options.Runtime == nil {
		return errors.New("CLI runtime support is unavailable")
	}
	rt := s.options.Runtime
	for _, worker := range workers {
		r, err := s.cliSnapshot(id)
		if err != nil {
			return err
		}
		if _, ok := r.CLI.Operation.Workers[worker]; !ok {
			running, err := rt.State(ctx, worker, KindCLI)
			s.mu.Lock()
			for _, b := range s.entries {
				if b.record.AgentID == worker && b.record.ResourceID == id && b.record.CLIApply != nil && b.record.CLIApply.ResumeRecorded {
					running = running || b.record.CLIApply.Resume
				}
			}
			s.mu.Unlock()
			if err != nil {
				if !s.hasCLIBinding(id, worker) {
					continue
				}
				return err
			}
			if err = s.editCLI(id, func(r *record) error {
				if r.CLI.Operation.Workers == nil {
					r.CLI.Operation.Workers = map[string]bool{}
				}
				r.CLI.Operation.Workers[worker] = running
				return nil
			}); err != nil {
				return err
			}
		}
		if err = rt.Update(ctx, worker, false, nil); err != nil {
			if !s.hasCLIBinding(id, worker) {
				continue
			}
			return err
		}
	}
	return nil
}

func (s *Service) applyCLIWorkers(ctx context.Context, id string) error {
	s.mu.Lock()
	var bindings []string
	for bid, e := range s.entries {
		if e.record.ResourceID != id || !bindingNeedsConfiguration(e.record) {
			continue
		}
		next := copyJSON(e.record)
		next.LastErrorCode = ""
		next.LastError = ""
		if next.CLIApply == nil {
			next.CLIApply = &cliBinding{}
		}
		next.CLIApply.Pending = true
		resource := s.entries[id]
		if resource != nil && resource.record.CLI.Operation != nil {
			if running, ok := resource.record.CLI.Operation.Workers[next.AgentID]; ok {
				next.CLIApply.Resume = running
				next.CLIApply.ResumeRecorded = true
			}
		}
		if err := s.persistLocked(bid, &next); err != nil {
			s.mu.Unlock()
			return err
		}
		e.record = next
		bindings = append(bindings, bid)
	}
	s.mu.Unlock()
	sort.Strings(bindings)
	var errs []error
	for _, bid := range bindings {
		if err := s.applyBinding(ctx, bid); err != nil {
			s.mu.Lock()
			exists := s.entries[bid] != nil
			s.mu.Unlock()
			if exists {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
func (s *Service) cliViewLocked(e *entry) Installation {
	item := view(e)
	resource := e
	if item.AgentID != "" {
		resource = s.entries[item.ResourceID]
	}
	if resource == nil {
		return item
	}
	r := resource.record
	item.AuthStatus = copyJSON(r.AuthStatus)
	item.Authorized = cliReady(r) && !r.Disconnected
	item.Revision = r.Revision
	if r.CLI != nil && r.CLI.Operation != nil {
		op := r.CLI.Operation.CLIOperation
		item.Operation = &op
	}
	if item.AgentID != "" {
		if e.record.CLIApply != nil && e.record.CLIApply.Pending {
			item.Status = "applying"
			if item.LastErrorCode != "" {
				item.Status = "error"
			}
		} else if !item.Enabled {
			item.Status = "disabled"
		} else if !cliReady(r) || r.Disconnected {
			item.Status = "needs_configuration"
		} else if item.AppliedRevision == r.Revision {
			item.Status = "connected"
		}
		return item
	}
	item.Migration = s.migrationViewLocked(item.InstallationID)
	if item.Migration != nil && item.Migration.Status != "completed" {
		item.LastErrorCode = "migration_failed"
	}
	item.ResourceID = item.InstallationID
	item.ResourceEnabled = true
	item.Status = "needs_configuration"
	if cliReady(r) && !r.Disconnected {
		item.Status = "connected"
	}
	if r.CLI != nil && activeCLI(r.CLI.Operation) {
		item.Status = "connecting"
	} else if item.LastErrorCode != "" {
		item.Status = "error"
	}
	item.Bindings = []BindingSummary{}
	for _, bound := range s.entries {
		if bound.record.ResourceID == item.InstallationID {
			b := s.cliViewLocked(bound)
			item.Bindings = append(item.Bindings, BindingSummary{InstallationID: b.InstallationID, AgentID: b.AgentID, Enabled: b.Enabled, Status: b.Status})
		}
	}
	sort.Slice(item.Bindings, func(i, j int) bool { return item.Bindings[i].AgentID < item.Bindings[j].AgentID })
	return item
}

func (s *Service) cliResourceSettledLocked(id string) bool {
	if s.cliMigration != nil && s.cliMigration.ResourceID == id && s.cliMigration.Status != "completed" {
		return false
	}
	e := s.entries[id]
	return e != nil && (e.record.Kind == KindMCP || (e.record.CLI != nil && (e.record.CLI.Operation == nil || e.record.CLI.Operation.Settled)))
}

func (s *Service) promoteQueuedCLI(id string) {
	if s.ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[id]
	if e == nil || e.record.CLI == nil || e.record.CLI.Queued == nil || activeCLI(e.record.CLI.Operation) {
		return
	}
	next := copyJSON(e.record)
	previous := next.CLI.Operation
	op := next.CLI.Queued
	op.Workers = map[string]bool{}
	if previous != nil {
		for id, running := range previous.Workers {
			op.Workers[id] = running
		}
		if !previous.Settled && next.CLI.Recovery == nil {
			next.CLI.Recovery = previous
		}
	}
	next.CLI.Operation = op
	next.CLI.Queued = nil
	if err := s.persistLocked(id, &next); err != nil {
		return
	}
	e.record = next
	s.scheduleConnectorWork()
}

func (s *Service) hasCLIBinding(resourceID, agentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		if e.record.ResourceID == resourceID && e.record.AgentID == agentID {
			return true
		}
	}
	return false
}

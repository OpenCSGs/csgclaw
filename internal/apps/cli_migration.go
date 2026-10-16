package apps

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sort"

	"csgclaw/internal/larkcli"
	"csgclaw/internal/localstore"
	agentruntime "csgclaw/internal/runtime"
	projection "csgclaw/internal/runtimeextension/larkcli"
	skill "csgclaw/internal/skill/state"
)

const cliMigrationSection = "feishu_cli_migration"

type LegacyWorker struct {
	AgentID    string                            `json:"agent_id"`
	Running    bool                              `json:"running"`
	ConfigFile string                            `json:"config_file,omitempty"`
	Extension  json.RawMessage                   `json:"extension,omitempty"`
	Projection *agentruntime.ExtensionProjection `json:"projection,omitempty"`
}
type CLIMigrationRuntime interface {
	RestoreLegacy(context.Context, string, json.RawMessage) error
	Verify(context.Context, string) (bool, error)
}
type migrationWorker struct {
	Recovered bool `json:"recovered,omitempty"`
	LegacyWorker
	BindingID           string                       `json:"binding_id"`
	Legacy              *projection.LegacyProjection `json:"legacy,omitempty"`
	Phase               string                       `json:"phase"`
	Committed           bool                         `json:"committed,omitempty"`
	VerificationPending bool                         `json:"verification_pending,omitempty"`
	Plan                *skill.SkillPlan             `json:"skill_plan,omitempty"`
	ErrorCode           string                       `json:"error_code,omitempty"`
}
type cliMigration struct {
	ResourceID   string                      `json:"resource_id,omitempty"`
	Selected     string                      `json:"selected,omitempty"`
	SourceDigest string                      `json:"source_digest,omitempty"`
	SourceConfig []byte                      `json:"source_config,omitempty"`
	Process      larkcli.ProcessRef          `json:"process,omitempty"`
	GlobalReady  bool                        `json:"global_ready,omitempty"`
	Status       string                      `json:"status"`
	Workers      map[string]*migrationWorker `json:"workers,omitempty"`
}
type CLIMigrationStatus struct {
	Status  string                     `json:"status"`
	Workers []CLIMigrationWorkerStatus `json:"workers"`
}
type CLIMigrationWorkerStatus struct {
	AgentID             string `json:"agent_id"`
	Phase               string `json:"phase"`
	ErrorCode           string `json:"error_code,omitempty"`
	VerificationPending bool   `json:"verification_pending"`
}

func (s *Service) CLIMigrationPrepared() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cliMigration != nil
}

// RestoreCLIMigration completes the saved configuration transition before
// automatic Agent restoration starts using the new connector configuration.
func (s *Service) RestoreCLIMigration(ctx context.Context) error {
	s.cliWorkMu.Lock()
	defer s.cliWorkMu.Unlock()
	m := s.migrationSnapshot()
	if m == nil || m.Status == "completed" {
		return nil
	}
	if m.Status == "waiting_for_legacy_callers" {
		return errors.New("Feishu CLI migration needs repair")
	}
	return s.executeMigration(ctx)
}

// PrepareCLIMigration runs before automatic Agent restoration. Selection and
// original running intent are persisted once, before any Runtime is changed.
func (s *Service) PrepareCLIMigration(ctx context.Context, candidates []LegacyWorker) error {
	s.cliWorkMu.Lock()
	defer s.cliWorkMu.Unlock()
	s.mu.Lock()
	existing := s.cliMigration
	s.mu.Unlock()
	if existing != nil {
		return nil
	}
	migration := &cliMigration{Status: "completed", Workers: map[string]*migrationWorker{}}
	if len(candidates) > 0 {
		resource, err := s.EnsureFeishuCLI(ctx)
		if err != nil {
			return err
		}
		migration.ResourceID = resource.InstallationID
		migration.Status = "pending"
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].AgentID < candidates[j].AgentID })
		var readable []LegacyWorker
		for _, candidate := range candidates {
			bid, err := newInstallationID()
			if err != nil {
				return err
			}
			migration.Workers[candidate.AgentID] = &migrationWorker{LegacyWorker: candidate, BindingID: bid, Phase: "original"}
			if _, err = larkcli.ReadConfigIdentity(candidate.ConfigFile); err == nil {
				readable = append(readable, candidate)
			}
		}
		if len(readable) > 0 {
			selected, err := rand.Int(rand.Reader, big.NewInt(int64(len(readable))))
			if err != nil {
				return err
			}
			candidate := readable[selected.Int64()]
			data, err := os.ReadFile(candidate.ConfigFile)
			if err != nil {
				return err
			}
			data, _, err = larkcli.SelectConfig(data)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			migration.Selected = candidate.AgentID
			migration.SourceConfig = data
			migration.SourceDigest = hex.EncodeToString(sum[:])
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.saveMigrationLocked(migration); err != nil {
		return err
	}
	s.cliMigration = migration
	return nil
}
func (s *Service) saveMigrationLocked(m *cliMigration) error {
	return localstore.UpdateObjectSection(s.path, cliMigrationSection, func(values map[string]json.RawMessage) error {
		raw, err := json.Marshal(m)
		if err != nil {
			return err
		}
		values["v1"] = raw
		return nil
	})
}
func (s *Service) editMigration(fn func(*cliMigration) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cliMigration == nil {
		return ErrNotFound
	}
	next := copyJSON(s.cliMigration)
	if err := fn(next); err != nil {
		return err
	}
	if err := s.saveMigrationLocked(next); err != nil {
		return err
	}
	s.cliMigration = next
	return nil
}
func (s *Service) migrationSnapshot() *cliMigration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyJSON(s.cliMigration)
}
func (s *Service) LegacyCLIProjection(agentID string) (*projection.Payload, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.cliMigration
	if m == nil {
		return nil, "", false
	}
	worker := m.Workers[agentID]
	if worker == nil || worker.Committed || (worker.Phase != "original" && worker.Phase != "restoring") || worker.Legacy == nil {
		return nil, "", false
	}
	identity, err := larkcli.ReadConfigIdentity(filepath.Join(worker.Legacy.Projection.Environment["LARKSUITE_CLI_CONFIG_DIR"], projection.WorkspaceName, projection.ConfigFileName))
	if err != nil {
		return nil, "", false
	}
	return &projection.Payload{AgentID: agentID, AppID: identity.AppID, Legacy: copyJSON(worker.Legacy)}, "legacy-" + m.SourceDigest, true
}
func (s *Service) executeMigration(ctx context.Context) error {
	m := s.migrationSnapshot()
	if m == nil || m.Status == "completed" {
		return nil
	}
	if s.options.Runtime == nil {
		return errors.New("migration runtime is unavailable")
	}
	if err := larkcli.RecoverProcess(m.Process); err != nil {
		return err
	}
	rt := s.options.Runtime
	legacyRT, ok := rt.(CLIMigrationRuntime)
	if !ok {
		return errors.New("migration runtime is unavailable")
	}
	if err := s.editMigration(func(m *cliMigration) error { m.Status = "running"; return nil }); err != nil {
		return err
	}
	fail := func(id string, err error) error {
		_ = s.editMigration(func(m *cliMigration) error {
			m.Status = "waiting_for_legacy_callers"
			if w := m.Workers[id]; w != nil {
				w.ErrorCode = "migration_failed"
			} else if id != "" {
				m.Status = "pending"
				s.scheduleConnectorWork()
			}
			return nil
		})
		return err
	}
	ids := make([]string, 0, len(m.Workers))
	for id := range m.Workers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if s.migrationSnapshot().Workers[id] == nil {
			continue
		}
		if err := rt.Update(ctx, id, false, nil); err != nil {
			if s.migrationSnapshot().Workers[id] == nil {
				continue
			}
			return fail(id, err)
		}
	}
	// Snapshot generations only after all old callers are stopped. The copies
	// contain configuration references; the official credential store is shared.
	for _, id := range ids {
		worker := s.migrationSnapshot().Workers[id]
		if worker == nil {
			continue
		}
		if worker.Legacy == nil && worker.Projection != nil {
			sum := sha256.Sum256([]byte(id + "/" + projection.Name))
			dir := filepath.Join(s.dataRoot, m.ResourceID, "migration", hex.EncodeToString(sum[:16]))
			saved, err := projection.SaveLegacyProjection(*worker.Projection, dir)
			if err != nil {
				return fail(id, err)
			}
			if err = s.editMigration(func(m *cliMigration) error {
				if w := m.Workers[id]; w != nil {
					w.Legacy = saved
				}
				return nil
			}); err != nil {
				return fail(id, err)
			}
		}
	}
	if err := s.prepareMigrationGlobal(ctx); err != nil {
		return fail(m.Selected, err)
	}
	// Create desired bindings once. Persisting a binding records the user's
	// intent; each migration worker has a separate configuration commit marker.
	for _, id := range ids {
		worker := s.migrationSnapshot().Workers[id]
		if worker == nil {
			continue
		}
		if worker.Committed {
			continue
		}
		if err := s.ensureMigrationBinding(id, worker.BindingID, m.ResourceID); err != nil {
			return fail(id, err)
		}
		if err := s.editMigration(func(m *cliMigration) error {
			if w := m.Workers[id]; w != nil {
				w.Phase = "switching"
				w.ErrorCode = ""
			}
			return nil
		}); err != nil {
			return fail(id, err)
		}
		if err := s.applyBinding(ctx, worker.BindingID); err != nil {
			restoreErr := s.restoreMigrationWorker(ctx, id, legacyRT)
			return fail(id, errors.Join(err, restoreErr))
		}
		if err := s.editMigration(func(m *cliMigration) error {
			w := m.Workers[id]
			if w == nil {
				return nil
			}
			w.Phase = "configured"
			if !w.Running {
				w.Committed = true
				w.VerificationPending = true
			}
			return nil
		}); err != nil {
			return fail(id, err)
		}
	}
	// Every legacy configuration has now switched or been explicitly disabled.
	// Only this point permits commands to use the shared credential refresh lock.
	for _, id := range ids {
		worker := s.migrationSnapshot().Workers[id]
		if worker == nil {
			continue
		}
		if !worker.Running {
			continue
		}
		if err := rt.Update(ctx, id, true, nil); err != nil {
			var stopErrors []error
			for _, other := range ids {
				if stopErr := rt.Update(context.WithoutCancel(ctx), other, false, nil); stopErr != nil {
					stopErrors = append(stopErrors, stopErr)
				}
			}
			if len(stopErrors) == 0 && !worker.Committed {
				stopErrors = append(stopErrors, s.restoreMigrationWorker(ctx, id, legacyRT))
			}
			return fail(id, errors.Join(append([]error{err}, stopErrors...)...))
		}
		if err := s.editMigration(func(m *cliMigration) error {
			w := m.Workers[id]
			if w == nil {
				return nil
			}
			w.Committed = true
			w.Phase = "completed"
			w.VerificationPending = false
			w.ErrorCode = ""
			return nil
		}); err != nil {
			return fail(id, err)
		}
	}
	if err := s.editMigration(func(m *cliMigration) error { m.Status = "completed"; return nil }); err != nil {
		return err
	}
	return s.cleanupMigration(ctx)
}
func (s *Service) ensureMigrationBinding(agentID, bid, resourceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cliMigration == nil || s.cliMigration.Workers[agentID] == nil {
		return ErrNotFound
	}
	resource := s.entries[resourceID]
	if resource == nil {
		return ErrNotFound
	}
	e := s.entries[bid]
	if e == nil {
		e = &entry{record: record{Installation: Installation{InstallationID: bid, AgentID: agentID, ResourceID: resourceID, Enabled: true, Kind: KindCLI}}}
	}
	next := copyJSON(e.record)
	hydrateBinding(&next, resource.record)
	if next.CLIApply == nil || !next.CLIApply.Pending {
		next.CLIApply = &cliBinding{Pending: true, ResumeRecorded: true}
	} else {
		next.CLIApply.Resume = false
		next.CLIApply.ResumeRecorded = true
	}
	next.LastError = ""
	next.LastErrorCode = ""
	next.Status = "applying"
	if err := s.persistLocked(bid, &next); err != nil {
		return err
	}
	e.record = next
	s.entries[bid] = e
	return nil
}
func (s *Service) prepareMigrationGlobal(ctx context.Context) error {
	m := s.migrationSnapshot()
	if m.GlobalReady {
		return nil
	}
	r, err := s.cliSnapshot(m.ResourceID)
	if err != nil {
		return err
	}
	if r.AuthStatus != nil && !r.Disconnected {
		return s.editMigration(func(m *cliMigration) error { m.GlobalReady = true; return nil })
	}
	if len(m.SourceConfig) == 0 {
		return s.editMigration(func(m *cliMigration) error { m.GlobalReady = true; return nil })
	}
	var options CLIOptions
	if s.options.CLI != nil {
		options = *s.options.CLI
	}
	pkg, err := s.installCLI(ctx, filepath.Join(s.dataRoot, m.ResourceID, "package"), s.packages["feishu"].CLI.Install, nil)
	if err != nil {
		return err
	}
	client := larkcli.Client{Executable: pkg.Executable, ConfigDir: r.CLI.ConfigDir, OperationID: "migration", Process: func(ref larkcli.ProcessRef) error {
		return s.editMigration(func(m *cliMigration) error { m.Process = ref; return nil })
	}}
	client = client.WithRunner(options.Run)
	_, identity, err := larkcli.SelectConfig(m.SourceConfig)
	if err != nil {
		return err
	}
	var status larkcli.Status
	// A completed official write can precede the CSGClaw metadata commit.
	// Reuse that configuration after restart, including any refreshed credentials.
	if current, readErr := client.Status(ctx, false); readErr == nil && current.AppID == identity.AppID {
		status = current
	} else {
		secret := ""
		var ref struct {
			Source string `json:"source"`
		}
		_ = json.Unmarshal(identity.Secret, &ref)
		if ref.Source != "keychain" {
			if json.Unmarshal(identity.Secret, &secret) != nil {
				if options.LegacySecret == nil {
					return errors.New("legacy app credential resolver is unavailable")
				}
				secret, err = options.LegacySecret(ctx, m.Selected, identity.AppID)
				if err != nil {
					return err
				}
			}
		}
		status, err = client.ImportConfig(ctx, m.SourceConfig, secret)
		if err != nil {
			return err
		}
	}
	if err = s.editCLI(m.ResourceID, func(r *record) error {
		r.CLI.Executable = pkg.Executable
		r.CLI.SkillsDir = pkg.SkillsDir
		r.AuthStatus, _ = json.Marshal(status)
		r.CLI.Ready = status.Identities.Bot.Available || status.Identities.User.Available
		r.PackageVersion = pkg.Version
		r.SkillVersion = s.packages["feishu"].CLI.Install.Skills.Version
		r.Revision++
		return nil
	}); err != nil {
		return err
	}
	return s.editMigration(func(m *cliMigration) error { m.GlobalReady = true; return nil })
}
func (s *Service) restoreMigrationWorker(ctx context.Context, id string, rt CLIMigrationRuntime) error {
	m := s.migrationSnapshot()
	worker := m.Workers[id]
	if worker == nil || worker.Committed {
		return nil
	}
	if err := s.editMigration(func(m *cliMigration) error {
		if w := m.Workers[id]; w != nil {
			w.Phase = "restoring"
		}
		return nil
	}); err != nil {
		return err
	}
	err := s.options.Runtime.Update(ctx, id, worker.Running, func(ctx context.Context) error {
		s.mu.Lock()
		binding := s.entries[worker.BindingID]
		var plan *skill.SkillPlan
		if binding != nil && binding.record.CLIApply != nil {
			plan = copyJSON(binding.record.CLIApply.Plan)
		}
		if plan == nil {
			plan = copyJSON(worker.Plan)
		}
		s.mu.Unlock()
		if plan != nil {
			reverse := skill.SkillPlan{Before: plan.After, After: plan.Before}
			if err := s.options.Runtime.ApplySkills(ctx, id, reverse); err != nil {
				return err
			}
		}
		if err := rt.RestoreLegacy(ctx, id, worker.Extension); err != nil {
			return err
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if binding := s.entries[worker.BindingID]; binding != nil {
			next := copyJSON(binding.record)
			if plan != nil {
				next.Skills = plan.Before
			}
			next.CLIApply = &cliBinding{Pending: true, ResumeRecorded: true}
			next.LastErrorCode = "migration_failed"
			if err := s.persistLocked(worker.BindingID, &next); err != nil {
				return err
			}
			binding.record = next
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.editMigration(func(m *cliMigration) error {
		if w := m.Workers[id]; w != nil {
			w.Phase = "original"
			w.Recovered = true
		}
		return nil
	})
}
func (s *Service) cleanupMigration(ctx context.Context) error {
	m := s.migrationSnapshot()
	if m == nil || m.Status != "completed" {
		return nil
	}
	rt, ok := s.options.Runtime.(CLIMigrationRuntime)
	if !ok {
		return nil
	}
	for id, worker := range m.Workers {
		if worker.VerificationPending {
			loaded, err := rt.Verify(ctx, id)
			if err != nil || !loaded {
				continue
			}
			if err = s.editMigration(func(m *cliMigration) error {
				if w := m.Workers[id]; w != nil {
					w.VerificationPending = false
					w.Phase = "completed"
				}
				return nil
			}); err != nil {
				return err
			}
		}
		if worker.Legacy != nil {
			if err := os.RemoveAll(worker.Legacy.Directory); err != nil {
				_ = s.editMigration(func(m *cliMigration) error {
					if w := m.Workers[id]; w != nil {
						w.ErrorCode = "cleanup_pending"
					}
					return nil
				})
				return err
			}
			if err := s.editMigration(func(m *cliMigration) error {
				if w := m.Workers[id]; w != nil {
					w.Legacy = nil
					w.ErrorCode = ""
				}
				return nil
			}); err != nil {
				return err
			}
		}
	}
	latest := s.migrationSnapshot()
	for _, worker := range latest.Workers {
		if worker.VerificationPending || worker.Legacy != nil {
			return nil
		}
	}
	if len(latest.SourceConfig) > 0 {
		return s.editMigration(func(m *cliMigration) error { m.SourceConfig = nil; m.Process = larkcli.ProcessRef{}; return nil })
	}
	return nil
}
func (s *Service) migrationViewLocked(resourceID string) *CLIMigrationStatus {
	m := s.cliMigration
	if m == nil || m.ResourceID != resourceID {
		return nil
	}
	view := &CLIMigrationStatus{Status: m.Status, Workers: []CLIMigrationWorkerStatus{}}
	for id, w := range m.Workers {
		view.Workers = append(view.Workers, CLIMigrationWorkerStatus{AgentID: id, Phase: w.Phase, ErrorCode: w.ErrorCode, VerificationPending: w.VerificationPending})
	}
	sort.Slice(view.Workers, func(i, j int) bool { return view.Workers[i].AgentID < view.Workers[j].AgentID })
	return view
}

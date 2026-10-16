package apps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"csgclaw/internal/connectorcli"
)

// A saved disconnect takes precedence over importing legacy configuration,
// including when startup resumes an interrupted operation.
func (s *Service) migrationDisconnectRequested() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.cliMigration
	if m == nil || m.Status == "completed" {
		return false
	}
	if m.Status == "disconnecting" {
		return true
	}
	e := s.entries[m.ResourceID]
	if e == nil || e.record.CLI == nil {
		return false
	}
	for _, op := range []*cliOperation{e.record.CLI.Operation, e.record.CLI.Queued} {
		if op != nil && op.Action == "disconnect" && !op.Settled {
			return true
		}
	}
	return false
}

func (s *Service) prepareMigrationDisconnect(ctx context.Context, id string) error {
	m := s.migrationSnapshot()
	if m == nil || m.ResourceID != id || m.Status == "completed" {
		return nil
	}
	if s.options.Runtime == nil {
		return errors.New("migration runtime is unavailable")
	}
	if err := s.editMigration(func(m *cliMigration) error {
		m.Status = "disconnecting"
		return nil
	}); err != nil {
		return err
	}
	if err := connectorcli.RecoverProcess(m.Process); err != nil {
		return err
	}
	if err := s.editMigration(func(m *cliMigration) error {
		m.Process = connectorcli.ProcessRef{}
		return nil
	}); err != nil {
		return err
	}
	ids := make([]string, 0, len(m.Workers))
	for worker := range m.Workers {
		ids = append(ids, worker)
	}
	sort.Strings(ids)
	for _, worker := range ids {
		w := s.migrationSnapshot().Workers[worker]
		if w == nil {
			continue
		}
		running, err := s.options.Runtime.State(ctx, worker, KindCLI)
		if err != nil {
			if s.migrationSnapshot().Workers[worker] == nil {
				continue
			}
			return err
		}
		// Migration may already have stopped this worker before its binding
		// existed. Retain that original intent through disconnect retries.
		if err := s.editCLI(id, func(r *record) error {
			if r.CLI.Operation.Workers == nil {
				r.CLI.Operation.Workers = map[string]bool{}
			}
			if _, recorded := r.CLI.Operation.Workers[worker]; !recorded {
				r.CLI.Operation.Workers[worker] = running || w.Running
			}
			return nil
		}); err != nil {
			return err
		}
		if err := s.options.Runtime.Update(ctx, worker, false, nil); err != nil {
			if s.migrationSnapshot().Workers[worker] == nil {
				continue
			}
			return err
		}
		// Every legacy worker must remove its extension, including workers
		// whose migration failed before creating a binding.
		if err := s.ensureMigrationBinding(worker, w.BindingID, id); err != nil {
			if s.migrationSnapshot().Workers[worker] == nil {
				continue
			}
			return err
		}
	}
	return s.recoverMigrationExecutable(ctx, id)
}

// Older interrupted imports can have written the shared configuration before
// saving the executable path. Recover the package so logout still runs.
func (s *Service) recoverMigrationExecutable(ctx context.Context, id string) error {
	r, err := s.cliSnapshot(id)
	if err != nil || r.CLI.Executable != "" {
		return err
	}
	if _, err := os.Stat(filepath.Join(r.CLI.ConfigDir, "config.json")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	pkg, err := s.installCLI(ctx, filepath.Join(s.dataRoot, id, "package"), r.Package.CLI.Install, nil)
	if err != nil {
		return err
	}
	return s.editCLI(id, func(r *record) error {
		r.CLI.Executable = pkg.Executable
		return nil
	})
}

func (s *Service) finishMigrationDisconnect(id string) error {
	m := s.migrationSnapshot()
	if m == nil || m.ResourceID != id || m.Status != "disconnecting" {
		return nil
	}
	// All bindings now have their CLI extension removed. Recovery copies and
	// saved credentials must no longer be imported on the next connection.
	if err := os.RemoveAll(filepath.Join(s.dataRoot, id, "migration")); err != nil {
		return err
	}
	return s.editMigration(func(m *cliMigration) error {
		*m = cliMigration{Status: "completed", Workers: map[string]*migrationWorker{}}
		return nil
	})
}

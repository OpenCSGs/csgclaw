package taskcore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	legacyRootFileName      = "root.json"
	legacyChildrenFileName  = "children.json"
	legacyApprovalsFileName = "approvals.json"
	legacyPresenceFileName  = "presence.json"
	legacyAggregateVersion  = 1
)

// legacyTaskAggregate was an intermediate room-task layout. Released Agent and
// Team records used the same root.json filename without aggregate_version and
// kept children, approvals, presence, and events in sidecar files.
type legacyTaskAggregate struct {
	Task
	AggregateVersion int            `json:"aggregate_version,omitempty"`
	Children         []Task         `json:"children,omitempty"`
	Events           []TaskEvent    `json:"events,omitempty"`
	Approvals        []TaskApproval `json:"approvals,omitempty"`
	Presence         []TaskPresence `json:"presence,omitempty"`
}

type legacyTaskMigration struct {
	dir      string
	record   taskRecord
	events   []TaskEvent
	rootPath string
}

type currentTaskUpgrade struct {
	path   string
	record taskRecord
}

// migrateLegacyTaskRecords upgrades every legacy taskcore snapshot before the
// service or ID allocator observes the store. tasks.json is the per-record
// migration marker, making retries safe after an interrupted startup. Legacy
// files remain in place so an operator can roll back the binary if necessary.
func migrateLegacyTaskRecords(root string) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	occupied := make(map[string]string)
	currentUpgrades := make([]currentTaskUpgrade, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		tasksPath := filepath.Join(dir, tasksFileName)
		if _, err := os.Stat(tasksPath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		var record taskRecord
		if err := readJSONFile(tasksPath, &record); err != nil {
			return fmt.Errorf("read current task record %s: %w", tasksPath, err)
		}
		unversioned := record.SchemaVersion == 0
		if err := validateTaskRecordVersion(&record); err != nil {
			return fmt.Errorf("read current task record %s: %w", tasksPath, err)
		}
		snapshot, err := snapshotFromRecord(entry.Name(), record)
		if err != nil {
			return fmt.Errorf("read current task record %s: %w", tasksPath, err)
		}
		if err := reserveSnapshotTaskIDs(occupied, snapshot); err != nil {
			return err
		}
		if unversioned {
			currentUpgrades = append(currentUpgrades, currentTaskUpgrade{path: tasksPath, record: record})
		}
	}

	migrations := make([]legacyTaskMigration, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, tasksFileName)); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		rootPath := filepath.Join(dir, legacyRootFileName)
		if _, err := os.Stat(rootPath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		snapshot, err := readLegacyTaskSnapshot(dir)
		if err != nil {
			return fmt.Errorf("migrate legacy task record %s: %w", rootPath, err)
		}
		record, err := recordFromSnapshot(snapshot)
		if err != nil {
			return fmt.Errorf("migrate legacy task record %s: %w", rootPath, err)
		}
		if err := reserveSnapshotTaskIDs(occupied, snapshot); err != nil {
			// Development builds briefly allocated new-format IDs from zero. A
			// mixed store from that window cannot preserve both meanings of the
			// same global ID. Keep the current record authoritative and leave the
			// legacy files untouched instead of blocking startup or overwriting it.
			slog.Warn("legacy task migration skipped due to task ID conflict", "root", snapshot.Root.ID, "error", err)
			continue
		}
		migrations = append(migrations, legacyTaskMigration{
			dir:      dir,
			record:   record,
			events:   cloneEvents(snapshot.Events),
			rootPath: rootPath,
		})
	}

	sort.Slice(currentUpgrades, func(i, j int) bool { return currentUpgrades[i].path < currentUpgrades[j].path })
	for _, upgrade := range currentUpgrades {
		if err := writeJSONFileAtomic(upgrade.path, upgrade.record); err != nil {
			return fmt.Errorf("stamp current task schema %s: %w", upgrade.path, err)
		}
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].dir < migrations[j].dir })
	for _, migration := range migrations {
		if err := writeTaskEventsAtomic(filepath.Join(migration.dir, eventsFileName), migration.events); err != nil {
			return fmt.Errorf("migrate legacy task events %s: %w", migration.rootPath, err)
		}
		if err := writeJSONFileAtomic(filepath.Join(migration.dir, tasksFileName), migration.record); err != nil {
			return fmt.Errorf("commit legacy task migration %s: %w", migration.rootPath, err)
		}
	}
	return nil
}

func readLegacyTaskSnapshot(dir string) (Snapshot, error) {
	var aggregate legacyTaskAggregate
	if err := readJSONFile(filepath.Join(dir, legacyRootFileName), &aggregate); err != nil {
		return Snapshot{}, err
	}
	switch aggregate.AggregateVersion {
	case legacyAggregateVersion:
		return Snapshot{
			Root:      aggregate.Task,
			Children:  aggregate.Children,
			Approvals: aggregate.Approvals,
			Presence:  aggregate.Presence,
			Events:    aggregate.Events,
		}, nil
	case 0:
		var children []Task
		if err := readOptionalJSONFile(filepath.Join(dir, legacyChildrenFileName), &children); err != nil {
			return Snapshot{}, err
		}
		var approvals []TaskApproval
		if err := readOptionalJSONFile(filepath.Join(dir, legacyApprovalsFileName), &approvals); err != nil {
			return Snapshot{}, err
		}
		var presence []TaskPresence
		if err := readOptionalJSONFile(filepath.Join(dir, legacyPresenceFileName), &presence); err != nil {
			return Snapshot{}, err
		}
		events, err := readLegacyTaskEvents(filepath.Join(dir, eventsFileName))
		if err != nil {
			return Snapshot{}, err
		}
		return Snapshot{
			Root:      aggregate.Task,
			Children:  children,
			Approvals: approvals,
			Presence:  presence,
			Events:    events,
		}, nil
	default:
		return Snapshot{}, fmt.Errorf("unsupported legacy task aggregate version %d", aggregate.AggregateVersion)
	}
}

func reserveSnapshotTaskIDs(occupied map[string]string, snapshot Snapshot) error {
	owner := strings.TrimSpace(snapshot.Root.ID)
	tasks := append([]Task{snapshot.Root}, snapshot.Children...)
	for _, task := range tasks {
		id := strings.TrimSpace(task.ID)
		if previous, found := occupied[id]; found && previous != owner {
			return fmt.Errorf("task migration conflict: task id %q belongs to both roots %q and %q", id, previous, owner)
		}
	}
	for _, task := range tasks {
		id := strings.TrimSpace(task.ID)
		occupied[id] = owner
	}
	return nil
}

func readOptionalJSONFile(path string, target any) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return readJSONFile(path, target)
}

func readLegacyTaskEvents(path string) ([]TaskEvent, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	events := make([]TaskEvent, 0)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var event TaskEvent
			if err := json.Unmarshal(bytes.TrimSpace(line), &event); err != nil {
				if errors.Is(readErr, io.EOF) {
					break
				}
				return nil, fmt.Errorf("decode %s: %w", path, err)
			}
			events = append(events, event)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return events, nil
}

func writeTaskEventsAtomic(path string, events []TaskEvent) error {
	buffer := bytes.NewBuffer(nil)
	encoder := json.NewEncoder(buffer)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return err
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".events-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(buffer.Bytes()); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

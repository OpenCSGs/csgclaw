package taskcore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type TaskIDAllocator struct {
	mu   sync.Mutex
	root string
	next int64
}

const legacyTaskIndexFileName = "index.json"

type legacyTaskIndexCounter struct {
	Counters struct {
		Task int64 `json:"task"`
	} `json:"counters"`
}

var taskIDAllocators = struct {
	sync.Mutex
	byRoot map[string]*TaskIDAllocator
}{
	byRoot: make(map[string]*TaskIDAllocator),
}

func NewMemoryTaskIDAllocator() *TaskIDAllocator {
	return &TaskIDAllocator{}
}

func newPersistentTaskIDAllocator(root string) (*TaskIDAllocator, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("task store root is required")
	}
	root = filepath.Clean(root)
	key := taskIDAllocatorKey(root)

	taskIDAllocators.Lock()
	defer taskIDAllocators.Unlock()
	if allocator := taskIDAllocators.byRoot[key]; allocator != nil {
		return allocator, nil
	}

	allocator := &TaskIDAllocator{root: root}
	if err := allocator.load(); err != nil {
		return nil, err
	}
	taskIDAllocators.byRoot[key] = allocator
	return allocator, nil
}

func taskIDAllocatorKey(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return filepath.Clean(root)
}

func (a *TaskIDAllocator) Next() (string, error) {
	if a == nil {
		return "", fmt.Errorf("task id allocator is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	previous := a.next
	a.next++
	if err := a.saveLocked(); err != nil {
		a.next = previous
		return "", err
	}
	return formatTaskIdentifier(a.next), nil
}

func (a *TaskIDAllocator) Peek(offset int) string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return formatTaskIdentifier(a.next + int64(offset) + 1)
}

func (a *TaskIDAllocator) Bump(id string) error {
	if a == nil {
		return fmt.Errorf("task id allocator is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	next := maxCounterFromIdentifier(id, "task-", a.next)
	if next == a.next {
		return nil
	}
	previous := a.next
	a.next = next
	if err := a.saveLocked(); err != nil {
		a.next = previous
		return err
	}
	return nil
}

func (a *TaskIDAllocator) load() error {
	if a.root == "" {
		return nil
	}
	sequencePath := filepath.Join(a.root, sequenceFileName)
	state, ok, err := readTaskSequence(sequencePath)
	if err != nil {
		return err
	}
	next, err := persistedTaskSequence(a.root)
	if err != nil {
		return err
	}
	if ok && state.LastTask > next {
		next = state.LastTask
	}
	// Persist the reconciled value during startup instead of waiting for the
	// next task creation. sequence.json is therefore the canonical source as
	// soon as this migration-capable version has opened the store. After the
	// supported migration window, a later release can remove the legacy
	// index/directory fallback safely.
	if !ok || state.SchemaVersion != currentSequenceVersion || state.LastTask != next {
		if err := writeTaskSequence(sequencePath, taskSequenceState{LastTask: next}); err != nil {
			return err
		}
	}
	a.next = next
	return nil
}

func (a *TaskIDAllocator) saveLocked() error {
	if a.root == "" {
		return nil
	}
	return writeTaskSequence(filepath.Join(a.root, sequenceFileName), taskSequenceState{LastTask: a.next})
}

func formatTaskIdentifier(value int64) string {
	return fmt.Sprintf("task-%d", value)
}

// persistedTaskSequence reconciles ID sources used before sequence.json became
// canonical. It includes every numeric ID in current flat records because a
// child task can have a higher ID than its task-N root directory.
func persistedTaskSequence(root string) (int64, error) {
	var next int64
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		next = maxCounterFromIdentifier(entry.Name(), "task-", next)
		tasksPath := filepath.Join(root, entry.Name(), tasksFileName)
		if _, err := os.Stat(tasksPath); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return 0, err
		}
		var record taskRecord
		if err := readJSONFile(tasksPath, &record); err != nil {
			return 0, fmt.Errorf("read task IDs from %s: %w", tasksPath, err)
		}
		if err := validateTaskRecordVersion(&record); err != nil {
			return 0, fmt.Errorf("read task IDs from %s: %w", tasksPath, err)
		}
		for _, task := range record.Tasks {
			next = maxCounterFromIdentifier(task.ID, "task-", next)
		}
	}
	legacyPath := filepath.Join(root, legacyTaskIndexFileName)
	if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
		return next, nil
	} else if err != nil {
		return 0, err
	}
	var legacy legacyTaskIndexCounter
	if err := readJSONFile(legacyPath, &legacy); err != nil {
		return 0, err
	}
	if legacy.Counters.Task < 0 {
		return 0, fmt.Errorf("legacy task id counter is invalid: %d", legacy.Counters.Task)
	}
	if legacy.Counters.Task > next {
		next = legacy.Counters.Task
	}
	return next, nil
}

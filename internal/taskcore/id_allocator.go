package taskcore

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

type TaskIDAllocator struct {
	mu   sync.Mutex
	root string
	next int64
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
	state, ok, err := readTaskSequence(filepath.Join(a.root, sequenceFileName))
	if err != nil {
		return err
	}
	if ok {
		a.next = state.LastTask
		return nil
	}
	entries, err := buildTaskIndex(a.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		a.next = maxCounterFromIdentifier(entry.ID, "task-", a.next)
	}
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

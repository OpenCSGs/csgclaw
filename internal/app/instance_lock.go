package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"csgclaw/internal/config"
)

var ErrAlreadyRunning = errors.New("csgclaw application is already running")

type InstanceLock struct {
	file *os.File
}

func AcquireInstanceLock() (*InstanceLock, error) {
	root, err := config.DefaultDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create application directory: %w", err)
	}

	path := filepath.Join(root, "runtime.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open application instance lock: %w", err)
	}
	if err := lockInstanceFile(file); err != nil {
		_ = file.Close()
		if isInstanceLockBusy(err) {
			return nil, fmt.Errorf("%w (lock: %s)", ErrAlreadyRunning, path)
		}
		return nil, fmt.Errorf("acquire application instance lock: %w", err)
	}

	if err := file.Truncate(0); err == nil {
		if _, err := file.Seek(0, 0); err == nil {
			_, _ = file.WriteString(strconv.Itoa(os.Getpid()) + "\n")
			_ = file.Sync()
		}
	}
	return &InstanceLock{file: file}, nil
}

func (l *InstanceLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	unlockErr := unlockInstanceFile(file)
	closeErr := file.Close()
	if unlockErr != nil {
		return fmt.Errorf("release application instance lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close application instance lock: %w", closeErr)
	}
	return nil
}

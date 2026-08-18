//go:build !windows

package main

import (
	"fmt"
	"io"
	"time"
)

func platformDependencies() coordinatorDependencies {
	unsupported := func() error {
		return fmt.Errorf("desktop update helper is only supported on Windows")
	}
	return coordinatorDependencies{
		openParent: func(uint32) (parentProcess, error) {
			return nil, unsupported()
		},
		runInstaller: func(string, io.Writer) (int, error) {
			return -1, unsupported()
		},
		relaunchApplication: func(string) (relaunchResult, error) {
			return relaunchResult{}, unsupported()
		},
		sleep: time.Sleep,
	}
}

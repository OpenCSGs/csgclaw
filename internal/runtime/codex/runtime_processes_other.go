//go:build !linux

package codex

const canFindUntrackedRuntimeProcesses = false

func stopRuntimeProcessesUsingDir(string) ([]int, error) {
	return nil, nil
}

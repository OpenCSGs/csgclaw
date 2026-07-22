//go:build !linux

package codex

func stopRuntimeProcessesUsingDir(string) ([]int, error) {
	return nil, nil
}

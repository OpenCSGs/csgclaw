//go:build !linux

package codex

func runtimeDirFilesystemType(string) (string, string) {
	return "unavailable", ""
}

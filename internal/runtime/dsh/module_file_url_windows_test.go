//go:build windows

package dsh

import "testing"

func TestWindowsModuleFileURL(t *testing.T) {
	for path, want := range map[string]string{
		`C:\agents\安装 100% #\memory.mjs`:        "file:///C:/agents/%E5%AE%89%E8%A3%85%20100%25%20%23/memory.mjs",
		`\\server\share\dsh\context-bridge.mjs`: "file://server/share/dsh/context-bridge.mjs",
	} {
		if got := moduleFileURL(path); got != want {
			t.Fatalf("moduleFileURL(%q) = %q, want %q", path, got, want)
		}
	}
}

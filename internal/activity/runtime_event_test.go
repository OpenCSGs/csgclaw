package activity

import "testing"

func TestTerminalToolStatus(t *testing.T) {
	for status, want := range map[string]string{
		"completed": "completed", "complete": "completed", "Success": "completed", "succeeded": "completed",
		" failed ": "failed", "error": "failed",
		"canceled": "canceled", "cancelled": "canceled", "declined": "canceled", "interrupted": "canceled",
		"": "", "started": "", "pending": "", "running": "", "in_progress": "", "unknown": "",
	} {
		if got := TerminalToolStatus(status); got != want {
			t.Errorf("TerminalToolStatus(%q) = %q, want %q", status, got, want)
		}
	}
}

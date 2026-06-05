package hostuser

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestRunUserMatchesProcessIDs(t *testing.T) {
	got, err := RunUser()
	if err != nil {
		if os.Getuid() < 0 || os.Getgid() < 0 {
			t.Skip("host uid/gid unavailable")
		}
		t.Fatalf("RunUser() error = %v", err)
	}
	uidStr, gidStr, ok := strings.Cut(got, ":")
	if !ok {
		t.Fatalf("RunUser() = %q, want uid:gid", got)
	}
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		t.Fatalf("parse uid %q: %v", uidStr, err)
	}
	gid, err := strconv.Atoi(gidStr)
	if err != nil {
		t.Fatalf("parse gid %q: %v", gidStr, err)
	}
	if uid != os.Getuid() {
		t.Fatalf("uid = %d, want %d", uid, os.Getuid())
	}
	if gid != os.Getgid() {
		t.Fatalf("gid = %d, want %d", gid, os.Getgid())
	}
}

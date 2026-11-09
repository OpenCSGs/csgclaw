//go:build windows

package connectorcli

import (
	"os"
	"strconv"
	"testing"
)

func TestProcessIdentityWithoutPowerShell(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	identity := processStarted(os.Getpid())
	ticks, err := strconv.ParseUint(identity, 10, 64)
	if err != nil || ticks < 621355968000000000 {
		t.Fatalf("invalid process creation identity: %q, %v", identity, err)
	}
	if processStarted(os.Getpid()) != identity {
		t.Fatal("process identity changed between reads")
	}
	if processStarted(0) != "" {
		t.Fatal("a nonexistent process received an identity")
	}
}

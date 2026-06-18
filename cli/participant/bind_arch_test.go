package participant

import (
	"os"
	"strings"
	"testing"
)

func TestParticipantBindCLIStaysOnPublicAPISurface(t *testing.T) {
	data, err := os.ReadFile("bind.go")
	if err != nil {
		t.Fatalf("read bind.go: %v", err)
	}
	source := string(data)
	if strings.Contains(source, `"csgclaw/internal/participant/feishubind"`) || strings.Contains(source, "feishubind.") {
		t.Fatalf("participant bind CLI should use API client calls directly, not internal feishubind helpers")
	}
}

package api

import (
	"os"
	"strings"
	"testing"
)

func TestFeishuRegistrationDoesNotUseTransportClientAdapter(t *testing.T) {
	data, err := os.ReadFile("feishu_registration.go")
	if err != nil {
		t.Fatalf("read feishu_registration.go: %v", err)
	}
	if strings.Contains(string(data), "NewLocalClient") {
		t.Fatalf("Feishu registration should call in-process binding logic directly, not a local transport client adapter")
	}
}

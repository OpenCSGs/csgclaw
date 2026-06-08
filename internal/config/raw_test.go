package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRawAcceptsMinimalConfig(t *testing.T) {
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "default.model"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["model"]
`
	if err := ValidateRaw([]byte(content)); err != nil {
		t.Fatalf("ValidateRaw() error = %v", err)
	}
}

func TestValidateRawRejectsInvalidConfig(t *testing.T) {
	if err := ValidateRaw([]byte("[llm]\nprovider = \"openai\"\n")); err == nil {
		t.Fatal("ValidateRaw() error = nil, want legacy section failure")
	}
}

func TestWriteRawFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	content := `[server]
listen_addr = "127.0.0.1:18080"
access_token = "secret"

[models]
default = "default.model"

[models.providers.default]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
models = ["model"]
`
	if err := WriteRawFile(path, []byte(content)); err != nil {
		t.Fatalf("WriteRawFile() error = %v", err)
	}
	got, err := ReadRawFile(path)
	if err != nil {
		t.Fatalf("ReadRawFile() error = %v", err)
	}
	if string(got) != content {
		t.Fatalf("ReadRawFile() = %q, want %q", string(got), content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 0600", info.Mode().Perm())
	}
}

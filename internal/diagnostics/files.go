package diagnostics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func TextMediaType(mediaType string) bool {
	value, _, _ := mime.ParseMediaType(mediaType)
	return strings.HasPrefix(value, "text/") || value == "application/json" || strings.HasSuffix(value, "+json") || value == "application/xml" || strings.HasSuffix(value, "+xml") || value == "application/javascript" || value == "application/x-yaml" || value == "application/yaml"
}
func (r *Record) CaptureTextFile(metadata any, content io.Reader) {
	if r == nil {
		return
	}
	data, err := io.ReadAll(io.LimitReader(content, maxBodyCaptureBytes+1))
	status := "available"
	if err != nil {
		status = "read_failed"
		r.Issue("file_read_failed")
	} else if len(data) > maxBodyCaptureBytes {
		status = "too_large"
		r.Issue("file_content_too_large")
		data = nil
	} else if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		status = "non_utf8_metadata_only"
		data = nil
	}
	r.Capture("execution.file", map[string]any{"metadata": metadata, "content_status": status, "text": string(data)})
}

// CaptureRuntimeFiles reads only explicitly named runtime instruction/config
// files. Credentials, arbitrary workspace documents and environment files are
// never discovered by recursively walking an Agent's workspace.
func (r *Record) CaptureRuntimeFiles(paths ...string) {
	if r == nil {
		return
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || !filepath.IsAbs(path) || seen[path] {
			continue
		}
		seen[path] = true
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			r.Issue("runtime_instruction_file_unavailable")
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			r.Issue("runtime_instruction_file_unavailable")
			continue
		}
		data, err := io.ReadAll(io.LimitReader(file, maxBodyCaptureBytes+1))
		file.Close()
		if err != nil || len(data) > maxBodyCaptureBytes {
			r.Issue("runtime_instruction_file_incomplete")
			continue
		}
		hash := sha256.Sum256(data)
		var content any = string(data)
		var parseErr error
		switch strings.ToLower(filepath.Ext(path)) {
		case ".toml":
			var parsed map[string]any
			parseErr = toml.Unmarshal(data, &parsed)
			content = parsed
		case ".yaml", ".yml":
			parseErr = yaml.Unmarshal(data, &content)
		case ".json":
			parseErr = json.Unmarshal(data, &content)
		}
		if parseErr != nil {
			content = "[unavailable: configuration parse failed]"
			r.Issue("runtime_configuration_unparseable")
		}
		r.Capture("runtime.instruction_file", map[string]any{"path": filepath.Clean(path), "size_bytes": info.Size(), "sha256": hex.EncodeToString(hash[:]), "content": content, "observation": "file_at_turn_start"})
	}
}

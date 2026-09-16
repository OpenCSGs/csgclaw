package apiclient

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestDownloadRoomAttachmentIntegrityAndCleanup(t *testing.T) {
	for _, kind := range []string{"valid", "empty", "hash", "short", "long", "interrupted", "missing-metadata", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			payload := []byte("verified bytes")
			if kind == "empty" {
				payload = nil
			}
			dir := t.TempDir()
			target := filepath.Join(dir, "attachment")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-CSGClaw-Caller-Agent") != "agent-worker" {
					t.Error("missing CLI identity")
				}
				if kind != "missing-metadata" {
					w.Header().Set("X-CSGClaw-File-Size", strconv.Itoa(len(payload)))
					w.Header().Set("X-CSGClaw-File-SHA256", fmt.Sprintf("%x", sha256.Sum256(payload)))
				}
				switch kind {
				case "hash":
					w.Write([]byte("modified bytes"))
				case "short":
					w.Write(payload[:3])
				case "long":
					w.Write(append(payload, '!'))
				case "interrupted":
					w.Header().Set("Content-Length", "100")
					w.Write(payload[:3])
				case "canceled":
					cancel()
				default:
					w.Write(payload)
				}
			}))
			defer srv.Close()
			c := New(srv.URL, "secret", srv.Client()).WithCallerAgentID("agent-worker")
			path, err := c.DownloadRoomAttachment(ctx, "room", "attachment", target)
			if kind == "valid" || kind == "empty" {
				if err != nil || path != target {
					t.Fatalf("download: %s %v", path, err)
				}
				got, err := os.ReadFile(path)
				if err != nil || string(got) != string(payload) {
					t.Fatalf("bytes: %q %v", got, err)
				}
				info, _ := os.Stat(path)
				if info.Mode().Perm() != 0600 {
					t.Fatalf("permissions: %o", info.Mode().Perm())
				}
			} else if err == nil {
				t.Fatal("accepted invalid download")
			} else if _, err = os.Lstat(target); !os.IsNotExist(err) {
				t.Fatal("failed download left output")
			}
			matches, _ := filepath.Glob(filepath.Join(dir, ".csgclaw-download-*"))
			if len(matches) != 0 {
				t.Fatalf("leaked temporary files: %v", matches)
			}
		})
	}
}

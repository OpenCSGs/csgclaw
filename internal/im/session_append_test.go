package im

import (
	"bytes"
	"context"
	"csgclaw/internal/diagnostics"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSessionJSONLRecentUpdateWithNewMessageAvoidsReadingAllHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "room.jsonl")
	var history bytes.Buffer
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&history, "{\"id\":\"history-%d\",\"content\":\"%s\"}\n", i, strings.Repeat("x", 512))
	}
	committed := history.Bytes()
	if err := writeFileIfChanged(path, committed, 0o600); err != nil {
		t.Fatal(err)
	}
	updated := bytes.Clone(committed)
	updated[len(updated)-4] = 'y'
	updated = append(updated, []byte("{\"id\":\"new\",\"content\":\"Added message\"}\n")...)
	record := diagnostics.New("").Begin("room", "source", "", "agent", "recent-update")
	operation := diagnostics.BeginOperation(diagnostics.WithRecord(context.Background(), record), "save")
	appended, err := appendSessionJSONL(path, updated, operation)
	operation.Finish()
	if err != nil || appended {
		t.Fatalf("existing message update used append: %v, %v", appended, err)
	}
	read := record.Snapshot().Performance["im.compare_read"]
	if read.Count != 1 || read.Bytes == 0 || read.Bytes > 32*1024 {
		t.Fatalf("recent update scanned older history: %+v", read)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, committed) {
		t.Fatalf("rejected append modified the committed history: %v", err)
	}
}

func TestSessionJSONLAppendsNewMessagesAndReplacesUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "room.jsonl")
	messages := []Message{{ID: "first", SenderID: AdminUserID, Content: "First", CreatedAt: time.Now().UTC()}}
	if err := saveMessagesJSONL(path, "room", messages); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	messages = append(messages, Message{ID: "second", SenderID: AdminUserID, Content: "Second", CreatedAt: time.Now().UTC()})
	if err := saveMessagesJSONL(path, "room", messages); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() <= before.Size() {
		t.Fatalf("new message did not append to the committed file: %v", err)
	}
	messages[0].Content = "Updated first message"
	if err := saveMessagesJSONL(path, "room", messages); err != nil {
		t.Fatal(err)
	}
	replaced, err := os.Stat(path)
	if err != nil || os.SameFile(after, replaced) {
		t.Fatalf("existing message update did not replace the snapshot: %v", err)
	}
	loaded, err := loadMessagesJSONL(path, "room")
	if err != nil || !reflect.DeepEqual(loaded, messages) {
		t.Fatalf("message order or updates changed: %+v, %v", loaded, err)
	}
}

func TestSessionJSONLRecoversInterruptedAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "room.jsonl")
	messages := []Message{{ID: "first", SenderID: AdminUserID, Content: "Committed", CreatedAt: time.Now().UTC()}}
	if err := saveMessagesJSONL(path, "room", messages); err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "legacy complete record without newline", data: bytes.TrimSuffix(committed, []byte("\n"))},
		{name: "incomplete last record", data: append(bytes.Clone(committed), []byte(`{"id":"unfinished"`)...)},
		{name: "malformed last record without newline", data: append(bytes.Clone(committed), []byte("{invalid}")...), wantErr: true},
		{name: "invalid complete record", data: append(bytes.Clone(committed), []byte("{invalid}\n")...), wantErr: true},
		{name: "invalid middle record", data: append(append(bytes.Clone(committed), []byte("{invalid}\n")...), committed...), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			loaded, err := loadMessagesJSONL(path, "room")
			if tc.wantErr {
				if err == nil {
					t.Fatal("accepted a malformed complete record")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(loaded, messages) {
				t.Fatalf("committed prefix was not preserved: %+v, %v", loaded, err)
			}
			next := append(loaded, Message{ID: "next", SenderID: AdminUserID, Content: "After recovery", CreatedAt: time.Now().UTC()})
			if err := saveMessagesJSONL(path, "room", next); err != nil {
				t.Fatal(err)
			}
			restored, err := loadMessagesJSONL(path, "room")
			if err != nil || !reflect.DeepEqual(restored, next) {
				t.Fatalf("retry duplicated or lost messages: %+v, %v", restored, err)
			}
		})
	}
}

func TestSessionJSONLAppendRepairsExternalEditsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "room.jsonl")
	messages := []Message{{ID: "first", SenderID: AdminUserID, Content: "Original", CreatedAt: time.Now().UTC()}}
	if err := saveMessagesJSONL(path, "room", messages); err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Replace(committed, []byte("Original"), []byte("Modified"), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	messages = append(messages, Message{ID: "second", SenderID: AdminUserID, Content: "Added", CreatedAt: time.Now().UTC()})
	if err := saveMessagesJSONL(path, "room", messages); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadMessagesJSONL(path, "room")
	if err != nil || !reflect.DeepEqual(loaded, messages) {
		t.Fatalf("external edit was not repaired: %+v, %v", loaded, err)
	}
	outside := filepath.Join(dir, "outside.jsonl")
	if err := os.WriteFile(outside, committed, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := saveMessagesJSONL(link, "room", messages); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(link)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("snapshot retained a symlink: %v", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || !bytes.Equal(data, committed) {
		t.Fatalf("append followed a symlink: %v", err)
	}
}

func BenchmarkSessionJSONLWrite(b *testing.B) {
	var history bytes.Buffer
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&history, "{\"id\":\"history-%d\",\"content\":\"%s\"}\n", i, strings.Repeat("x", 512))
	}
	for _, mode := range []string{"atomic-replacement", "prefix-append"} {
		b.Run(mode, func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "room.jsonl")
			data := bytes.Clone(history.Bytes())
			if err := writeFileIfChanged(path, data, 0o600); err != nil {
				b.Fatal(err)
			}
			var written int64
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				line := []byte(fmt.Sprintf("{\"id\":\"new-%d\",\"content\":\"Added message\"}\n", i))
				data = append(data, line...)
				if mode == "prefix-append" {
					appended, err := appendSessionJSONL(path, data)
					if err != nil || !appended {
						b.Fatalf("append=%v error=%v", appended, err)
					}
					written += int64(len(line))
				} else {
					if err := writeFileIfChanged(path, data, 0o600); err != nil {
						b.Fatal(err)
					}
					written += int64(len(data))
				}
			}
			b.ReportMetric(float64(written)/float64(b.N), "written-B/op")
		})
	}
}

package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/im"
)

func readDiagnosticZIP(t *testing.T, response *http.Response) []byte {
	t.Helper()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/zip" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("invalid diagnostic ZIP response: status=%d type=%s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 1 || archive.File[0].Method != zip.Deflate {
		t.Fatal("expected one compressed JSON entry")
	}
	entry, err := archive.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer entry.Close()
	body, err := io.ReadAll(entry)
	if err != nil {
		t.Fatal(err)
	}
	var metadata diagnosticZIPMetadata
	if err := json.Unmarshal([]byte(archive.Comment), &metadata); err != nil {
		t.Fatal(err)
	}
	var bundle struct {
		SchemaVersion int    `json:"schema_version"`
		TurnID        string `json:"turn_id"`
		Collection    struct {
			Complete bool `json:"complete"`
		} `json:"collection"`
	}
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatal(err)
	}
	if metadata.SchemaVersion != bundle.SchemaVersion || metadata.TurnID != bundle.TurnID || metadata.Complete != bundle.Collection.Complete {
		t.Fatal("ZIP status metadata differs from complete JSON")
	}
	return body
}

func TestDiagnosticDownloadIsZIPWithExpandedJSON(t *testing.T) {
	service := im.NewServiceFromBootstrap(im.Bootstrap{CurrentUserID: im.AdminUserID, Users: []im.User{{ID: im.AdminUserID, Role: "admin"}}, Rooms: []im.Room{{ID: "room", Members: []string{im.AdminUserID}}}})
	record := service.Diagnostics().Begin("room", "source", "", "manager", "turn-csgclaw-zip-test")
	record.Capture("model.runtime_request", map[string]any{"body": map[string]any{"tools": []any{map[string]string{"name": "lookup", "description": "complete tool definition"}}, "input": "full request"}})
	record.Finish("succeeded")
	server := httptest.NewServer((&Handler{im: service}).Routes())
	defer server.Close()
	response, err := http.Get(server.URL + "/api/v1/rooms/room/diagnostics/" + record.Snapshot().ID + "/bundle?download=1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("download must be a ZIP: status=%d type=%s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 1 || archive.File[0].Name != "csgclaw-turn-zip-test-diagnostics.json" {
		t.Fatal("ZIP must contain exactly the full diagnostic JSON")
	}
	if !strings.Contains(response.Header.Get("Content-Disposition"), `filename="csgclaw-turn-zip-test-diagnostics.zip"`) {
		t.Fatal("incorrect ZIP filename")
	}
	response.Body = io.NopCloser(bytes.NewReader(raw))
	body := readDiagnosticZIP(t, response)
	for _, expected := range []string{"complete tool definition", "full request", `"schema_version": 1`, `"turn_id": "turn-csgclaw-zip-test"`} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("ZIP dropped diagnostic context: %s", expected)
		}
	}
	if bytes.Contains(body, []byte(`"deduplication"`)) {
		t.Fatal("export introduced reference deduplication")
	}
	copyResponse, err := http.Get(server.URL + "/api/v1/rooms/room/diagnostics/" + record.Snapshot().ID + "/bundle")
	if err != nil {
		t.Fatal(err)
	}
	defer copyResponse.Body.Close()
	if copyResponse.Header.Get("Content-Type") != "application/json; charset=utf-8" || copyResponse.Header.Get("Content-Disposition") != "" {
		t.Fatal("copy endpoint must remain plain JSON")
	}
	copyBody, err := io.ReadAll(copyResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"complete tool definition", "full request"} {
		if !bytes.Contains(copyBody, []byte(expected)) {
			t.Fatal("copy endpoint lost full context")
		}
	}
	for _, test := range []struct {
		room, caller string
		status       int
	}{{"other", "", 404}, {"room", "agent-outsider", 403}} {
		request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/rooms/"+test.room+"/diagnostics/"+record.Snapshot().ID+"/bundle?download=1", nil)
		request.Header.Set("X-CSGClaw-Caller-Agent", test.caller)
		r, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != test.status || r.Header.Get("Content-Type") == "application/zip" {
			t.Fatalf("ZIP access control: got %d want %d", r.StatusCode, test.status)
		}
	}
}

func TestDiagnosticZIPStreamsAndPreservesExactJSON(t *testing.T) {
	data := make([]byte, 256<<10)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	prefix := []byte(`{"record_id":9007199254740993,"body":"` + hex.EncodeToString(data))
	suffix := []byte(`"}` + "\n")
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	done := make(chan error, 1)
	go func() {
		err := writeDiagnosticZIP(writer, "diagnostics.json", "turn-stream", func(out io.Writer) (bool, error) {
			if _, err := out.Write(prefix); err != nil {
				return false, err
			}
			<-release
			_, err := out.Write(suffix)
			return true, err
		})
		writer.CloseWithError(err)
		done <- err
	}()
	first := make([]byte, 32<<10)
	received := make(chan error, 1)
	go func() { _, err := io.ReadFull(reader, first); received <- err }()
	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("export buffered JSON instead of streaming ZIP bytes")
	}
	select {
	case err := <-done:
		t.Fatalf("producer finished before JSON was complete: %v", err)
	default:
	}
	close(release)
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	raw := append(first, rest...)
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := archive.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer entry.Close()
	body, err := io.ReadAll(entry)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, append(prefix, suffix...)) {
		t.Fatal("ZIP changed original JSON bytes or numeric precision")
	}
}

func TestDiagnosticZIPRejectsInterruptedCollection(t *testing.T) {
	var out bytes.Buffer
	err := writeDiagnosticZIP(&out, "diagnostics.json", "turn", func(w io.Writer) (bool, error) {
		_, _ = io.WriteString(w, `{"partial":`)
		return false, context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("collection failure lost: %v", err)
	}
	if _, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len())); err == nil {
		t.Fatal("interrupted export finalized a valid ZIP")
	}
}

func TestDiagnosticZIPLargeSample(t *testing.T) {
	path := os.Getenv("CSGCLAW_DIAGNOSTIC_ZIP_SAMPLE")
	if path == "" {
		t.Skip("private large diagnostic sample not supplied")
	}
	input, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(filepath.Join(t.TempDir(), "diagnostics.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	original := sha256.New()
	err = writeDiagnosticZIP(output, filepath.Base(path), "turn-sample", func(w io.Writer) (bool, error) {
		_, err := io.Copy(io.MultiWriter(w, original), input)
		return false, err
	})
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := output.Stat()
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(output, compressed.Size())
	if err != nil {
		t.Fatal(err)
	}
	entry, err := archive.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer entry.Close()
	restored := sha256.New()
	n, err := io.Copy(restored, entry)
	if err != nil {
		t.Fatal(err)
	}
	if n != info.Size() || !bytes.Equal(original.Sum(nil), restored.Sum(nil)) {
		t.Fatal("large sample changed after streaming ZIP round trip")
	}
	t.Logf("original=%d ZIP=%d bytes; exact SHA-256 match", n, compressed.Size())
}

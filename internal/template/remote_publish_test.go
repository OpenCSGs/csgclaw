package template

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func respondTemplatePreupload(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	var request struct {
		Files []remotePreuploadFile `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Error(err)
	}
	var response struct {
		Data struct {
			Files []remotePreuploadResult `json:"files"`
		} `json:"data"`
	}
	for _, file := range request.Files {
		response.Data.Files = append(response.Data.Files, remotePreuploadResult{Path: file.Path, UploadMode: "regular"})
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Error(err)
	}
}

func TestRemoteTemplateBatchPublication(t *testing.T) {
	for _, scenario := range []string{"create", "update", "lookup_error", "conflict", "metadata_error", "preupload_error", "commit_error", "missing_result", "lfs", "ignored"} {
		t.Run(scenario, func(t *testing.T) {
			var events []string
			var committed []remoteCommitFile
			var updatedNickname string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer token" || (r.Method != http.MethodPut && r.URL.Query().Get("current_user") != "wanghj") {
					t.Error("missing publication credentials")
				}
				if r.Method == http.MethodDelete {
					t.Error("publication must not delete repository")
				}
				events = append(events, r.Method+" "+r.URL.Path)
				switch r.URL.Path {
				case "/api/v1/codes/wanghj/ReviewBot":
					if r.Method == http.MethodPut {
						var request remoteUpdateCodeRequest
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
						}
						updatedNickname = request.Nickname
						if scenario == "metadata_error" {
							http.Error(w, "update failed", http.StatusBadRequest)
							return
						}
						w.WriteHeader(http.StatusOK)
						return
					}
					switch scenario {
					case "lookup_error":
						http.Error(w, "unavailable", 500)
					case "update", "conflict", "metadata_error":
						_, _ = w.Write([]byte(`{"data":{"default_branch":"release"}}`))
					default:
						http.NotFound(w, r)
					}
				case "/api/v1/codes":
					var request remoteCreateCodeRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if request.CodeFile != "" {
						t.Error("creation must not use archive")
					}
					_, _ = w.Write([]byte(`{"data":{"path":"wanghj/ReviewBot"}}`))
				case "/api/v1/codes/wanghj/ReviewBot/last_commit":
					_, _ = w.Write([]byte(`{"data":{"id":"base"}}`))
				case "/api/v1/codes/wanghj/ReviewBot/refs/base/tree/":
					_, _ = w.Write([]byte(`{"data":{"files":[{"path":"obsolete.txt","type":"file"},{"path":".gitattributes","type":"file"}],"cursor":""}}`))
				case "/api/v1/codes/wanghj/ReviewBot/preupload/main", "/api/v1/codes/wanghj/ReviewBot/preupload/base":
					if scenario == "preupload_error" {
						http.Error(w, "denied", 403)
						return
					}
					if scenario == "missing_result" {
						_, _ = w.Write([]byte(`{"data":{"files":[]}}`))
						return
					}
					var request struct {
						Files []remotePreuploadFile `json:"files"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					var response struct {
						Data struct {
							Files []remotePreuploadResult `json:"files"`
						} `json:"data"`
					}
					for _, file := range request.Files {
						if _, err := base64.StdEncoding.DecodeString(file.Sample); err != nil {
							t.Error(err)
						}
						result := remotePreuploadResult{Path: file.Path, UploadMode: "regular"}
						if scenario == "update" {
							result.OID = "existing-object"
						}
						if scenario == "lfs" {
							result.UploadMode = "lfs"
						}
						result.ShouldIgnore = scenario == "ignored"
						result.IsDir = scenario == "directory"
						response.Data.Files = append(response.Data.Files, result)
					}
					_ = json.NewEncoder(w).Encode(response)
				default:
					if r.URL.Path != "/api/v1/codes/wanghj/ReviewBot/commit/main" && r.URL.Path != "/api/v1/codes/wanghj/ReviewBot/commit/release" {
						t.Errorf("unexpected request: %s", r.URL)
						http.NotFound(w, r)
						return
					}
					if scenario == "commit_error" {
						http.Error(w, "commit failed", 400)
						return
					}
					var request struct {
						Files []remoteCommitFile `json:"files"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					committed = append(committed, request.Files...)
					w.WriteHeader(200)
				}
			}))
			defer server.Close()
			remote := NewAuthenticatedRemoteStore(server.URL, "token", "wanghj")
			spec := versionSpec(t, "ReviewBot")
			if scenario == "update" {
				spec.DisplayName = "Updated Reviewer"
			}
			spec.ReplaceRemote = scenario != "conflict"
			_, err := remote.Publish(context.Background(), spec)
			wantSuccess := scenario == "create" || scenario == "update"
			if (err == nil) != wantSuccess {
				t.Fatalf("Publish error = %v", err)
			}
			if scenario == "create" {
				want := []string{"GET /api/v1/codes/wanghj/ReviewBot", "POST /api/v1/codes", "POST /api/v1/codes/wanghj/ReviewBot/preupload/main", "POST /api/v1/codes/wanghj/ReviewBot/commit/main", "POST /api/v1/codes/wanghj/ReviewBot/commit/main"}
				if !reflect.DeepEqual(events, want) {
					t.Fatalf("events=%v", events)
				}
			}
			if scenario == "update" {
				if updatedNickname != "Updated Reviewer" {
					t.Fatalf("updated nickname=%q", updatedNickname)
				}
			}
			if scenario == "lookup_error" || scenario == "conflict" {
				if len(events) != 1 {
					t.Fatalf("unexpected mutation: %v", events)
				}
			}
			if !wantSuccess {
				for _, file := range committed {
					if file.Path == localManifestFileName {
						t.Fatal("failed publication committed the version marker")
					}
				}
				if len(committed) > 0 && scenario != "metadata_error" {
					t.Fatal("unexpected commit")
				}
				return
			}
			foundManifest, foundDelete := false, false
			for _, file := range committed {
				if file.Path == ".gitattributes" {
					t.Error("removed Git policy")
				}
				if file.Path == "obsolete.txt" {
					foundDelete = file.Action == "delete"
					continue
				}
				action := "create"
				if scenario == "update" {
					action = "update"
				}
				if file.Action != action {
					t.Errorf("%s action=%s", file.Path, file.Action)
				}
				content, err := base64.StdEncoding.DecodeString(file.Content)
				if err != nil {
					t.Fatal(err)
				}
				if file.Path == "agent.toml" {
					foundManifest = strings.Contains(string(content), "ReviewBot")
				}
			}
			if !foundManifest {
				t.Fatal("missing template manifest")
			}
			if scenario == "update" && !foundDelete {
				t.Fatal("stale file not deleted")
			}
		})
	}
}

func TestRemotePublicationFilesTraversesPagesAndDirectories(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path+"?"+r.URL.Query().Get("cursor"))
		switch {
		case strings.HasSuffix(r.URL.Path, "/tree/skills"):
			_, _ = w.Write([]byte(`{"data":{"files":[{"path":"skills/old.txt","type":"file"}]}}`))
		case r.URL.Query().Get("cursor") == "next":
			_, _ = w.Write([]byte(`{"data":{"files":[{"path":"agent.toml","type":"file"}]}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"files":[{"path":"skills","type":"dir"}],"cursor":"next"}}`))
		}
	}))
	defer server.Close()
	remote := NewAuthenticatedRemoteStore(server.URL, "token", "alice")
	files := make(map[string]string)
	if err := remote.remotePublicationFiles(context.Background(), "Reviewer", "main", "", files); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || !reflect.DeepEqual(files, map[string]string{"agent.toml": "", "skills/old.txt": ""}) {
		t.Fatalf("calls=%v files=%v", calls, files)
	}
}

func TestRemotePublicationAllowsCombinedSizeAbovePerFileLimit(t *testing.T) {
	spec := versionSpec(t, "Reviewer")
	files, err := buildRemoteTemplateFiles(spec, remotePublishMaxFileBytes)
	if err != nil {
		t.Fatal(err)
	}
	limit := len(files[localManifestFileName])
	if err := os.WriteFile(filepath.Join(spec.WorkspaceRef.Path, "AGENTS.md"), make([]byte, limit), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err = buildRemoteTemplateFiles(spec, int64(limit))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(files["instructions/AGENTS.md"]); got != limit {
		t.Fatalf("instructions file size = %d, want %d", got, limit)
	}
}

func TestRemotePublicationFileLimitPrecedesRepositoryCreation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "unexpected request", 500) }))
	defer server.Close()
	remote := NewAuthenticatedRemoteStore(server.URL, "token", "alice")
	spec := versionSpec(t, "Reviewer")
	if err := os.Truncate(filepath.Join(spec.WorkspaceRef.Path, "AGENTS.md"), remotePublishMaxFileBytes+1); err != nil {
		t.Fatal(err)
	}
	_, err := remote.Publish(context.Background(), spec)
	var tooLarge *PublishFileTooLargeError
	if !errors.As(err, &tooLarge) || tooLarge.Path != "instructions/AGENTS.md" || tooLarge.MaxBytes != remotePublishMaxFileBytes {
		t.Fatalf("expected size limit error for instructions/AGENTS.md, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("oversized snapshot made %d remote requests", calls)
	}
}

func TestRemotePublicationTotalLimitPrecedesRepositoryCreation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("oversized snapshot made remote request %s", r.URL)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	defer server.Close()
	remote := NewAuthenticatedRemoteStore(server.URL, "token", "alice")
	spec := versionSpec(t, "Reviewer")
	dir := filepath.Join(spec.WorkspaceRef.Path, "skills", "review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The files alone reach the limit. The manifest and instructions count too.
	for i := 0; i < remotePublishMaxTotalBytes/remotePublishMaxFileBytes; i++ {
		name := filepath.Join(dir, fmt.Sprintf("data-%02d.txt", i))
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Truncate(name, remotePublishMaxFileBytes); err != nil {
			t.Fatal(err)
		}
	}
	_, err := remote.Publish(t.Context(), spec)
	var tooLarge *PublishTotalTooLargeError
	if !errors.As(err, &tooLarge) || tooLarge.MaxBytes != remotePublishMaxTotalBytes {
		t.Fatalf("expected total size limit error, got %v", err)
	}
}

func TestRemotePublicationCommitsSizeAndCountBoundedBatches(t *testing.T) {
	files := map[string][]byte{
		"data-a":              make([]byte, remotePublishMaxBatchBytes+1),
		"data-b":              []byte("small file"),
		localManifestFileName: []byte("manifest"),
	}
	for i := 0; i < remotePublishMaxBatchFiles; i++ {
		files[fmt.Sprintf("data-c-%03d", i)] = nil
	}
	var preuploads, commits []int
	committed := make(map[string]bool)
	var manifestCommitted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/preupload/"):
			var request struct{ Files []remotePreuploadFile }
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			preuploads = append(preuploads, len(request.Files))
			var size int
			var results []remotePreuploadResult
			for _, file := range request.Files {
				size += file.Size
				results = append(results, remotePreuploadResult{Path: file.Path, UploadMode: "regular"})
			}
			if (size > remotePublishMaxBatchBytes && len(request.Files) != 1) || len(request.Files) > remotePublishMaxBatchFiles {
				t.Errorf("oversized preupload batch with %d files and %d bytes", len(request.Files), size)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"files": results}})
		case strings.Contains(r.URL.Path, "/commit/"):
			var request struct{ Files []remoteCommitFile }
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				return
			}
			if len(preuploads) != 3 || !strings.HasSuffix(r.URL.Path, "/main") {
				t.Errorf("started committing before all preuploads: %v", preuploads)
			}
			commits = append(commits, len(request.Files))
			var size int
			for _, file := range request.Files {
				if file.Path == localManifestFileName {
					manifestCommitted = len(request.Files) == 1 && len(committed) == len(files)-1
				}
				content, err := base64.StdEncoding.DecodeString(file.Content)
				if err != nil || len(content) != len(files[file.Path]) || file.Action != "create" || committed[file.Path] {
					t.Errorf("invalid commit operation for %q", file.Path)
				}
				size += len(content)
				committed[file.Path] = true
			}
			if (size > remotePublishMaxBatchBytes && len(request.Files) != 1) || len(request.Files) > remotePublishMaxBatchFiles {
				t.Errorf("oversized commit batch with %d files and %d bytes", len(request.Files), size)
			}
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	remote := NewAuthenticatedRemoteStore(server.URL, "token", "alice")
	if err := remote.commitRemoteTemplate(t.Context(), "Reviewer", "main", files, false, "Reviewer"); err != nil {
		t.Fatal(err)
	}
	want := []int{1, remotePublishMaxBatchFiles, 2}
	if !reflect.DeepEqual(preuploads, want) || !reflect.DeepEqual(commits, []int{1, remotePublishMaxBatchFiles, 1, 1}) || len(committed) != len(files) || !manifestCommitted {
		t.Fatalf("preuploads=%v commits=%v committed=%d manifest committed last=%t", preuploads, commits, len(committed), manifestCommitted)
	}
}

func TestRemoteCommitReplacesConflictingFileAndDirectory(t *testing.T) {
	for _, directoryToFile := range []bool{true, false} {
		t.Run(fmt.Sprint(directoryToFile), func(t *testing.T) {
			oldPath, newPath := "instructions/foo/bar", "instructions/foo"
			if !directoryToFile {
				oldPath, newPath = newPath, oldPath
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/last_commit"):
					_, _ = w.Write([]byte(`{"data":{"id":"base"}}`))
				case strings.Contains(r.URL.Path, "/tree/"):
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"files": []map[string]string{{"path": oldPath, "type": "file"}}}})
				case strings.Contains(r.URL.Path, "/preupload/"):
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"files": []remotePreuploadResult{{Path: newPath, UploadMode: "regular", IsDir: directoryToFile}}}})
				case strings.Contains(r.URL.Path, "/commit/"):
					var request struct {
						Files []remoteCommitFile `json:"files"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if len(request.Files) != 2 || request.Files[0].Path != oldPath || request.Files[0].Action != "delete" || request.Files[1].Path != newPath || request.Files[1].Action != "create" {
						t.Errorf("invalid commit order: %+v", request.Files)
					}
				default:
					t.Errorf("unexpected request %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			remote := NewAuthenticatedRemoteStore(server.URL, "token", "alice")
			if err := remote.commitRemoteTemplate(context.Background(), "Reviewer", "main", map[string][]byte{newPath: []byte("content")}, true, "Reviewer"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRemotePublicationRetryCommitsOnlyRemainingFilesAndManifestLast(t *testing.T) {
	for _, scenario := range []string{"data_timeout", "manifest_response_lost", "manual_retry", "initial_head_unavailable", "initial_tree_unavailable", "recovery_tree_unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			files := map[string][]byte{localManifestFileName: []byte("new"), "unchanged.txt": []byte("same")}
			remoteFiles := map[string][]byte{localManifestFileName: []byte("old"), "unchanged.txt": []byte("same")}
			for i := 0; i < remotePublishMaxBatchFiles+1; i++ {
				remoteFiles[fmt.Sprintf("stale-%03d", i)] = []byte("stale")
			}
			commits, reads := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/last_commit"):
					reads++
					if scenario == "initial_head_unavailable" && reads == 1 {
						http.Error(w, "head temporarily unavailable", http.StatusServiceUnavailable)
						return
					}
					_, _ = w.Write([]byte(`{"data":{"id":"base"}}`))
				case strings.Contains(r.URL.Path, "/tree/"):
					if (scenario == "initial_tree_unavailable" && reads == 1) ||
						(scenario == "recovery_tree_unavailable" && reads == 2) {
						http.Error(w, "tree temporarily unavailable", http.StatusServiceUnavailable)
						return
					}
					entries := make([]map[string]string, 0, len(remoteFiles))
					for path, content := range remoteFiles {
						oid := sha1.Sum(fmt.Appendf(nil, "blob %d\x00%s", len(content), content))
						entries = append(entries, map[string]string{"path": path, "type": "file", "sha": fmt.Sprintf("%x", oid)})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"files": entries}})
				case strings.Contains(r.URL.Path, "/preupload/"):
					respondTemplatePreupload(t, w, r)
				case strings.HasSuffix(r.URL.Path, "/commit/main"):
					commits++
					var request struct{ Files []remoteCommitFile }
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					if len(request.Files) > remotePublishMaxBatchFiles {
						t.Error("oversized commit")
					}
					if commits == 2 && (scenario == "data_timeout" || scenario == "manual_retry" || scenario == "recovery_tree_unavailable") {
						if string(remoteFiles[localManifestFileName]) != "old" {
							t.Error("manifest changed before all data batches succeeded")
						}
						status := http.StatusGatewayTimeout
						if scenario == "manual_retry" {
							status = http.StatusBadRequest
						}
						http.Error(w, "failed before commit", status)
						return
					}
					for _, file := range request.Files {
						switch file.Path {
						case "unchanged.txt":
							t.Error("unchanged file was committed")
						case localManifestFileName:
							if len(request.Files) != 1 || len(remoteFiles) != 2 {
								t.Error("manifest was committed before remaining files")
							}
							remoteFiles[file.Path], _ = base64.StdEncoding.DecodeString(file.Content)
						default:
							if _, exists := remoteFiles[file.Path]; !exists || file.Action != "delete" {
								t.Errorf("replayed operation: %+v", file)
							}
							delete(remoteFiles, file.Path)
						}
					}
					if commits == 3 && scenario == "manifest_response_lost" {
						http.Error(w, "response lost after manifest commit", http.StatusGatewayTimeout)
					}
				case r.Method == http.MethodPut:
					if len(remoteFiles) != 2 {
						t.Error("metadata changed before all data batches succeeded")
					}
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			remote := NewAuthenticatedRemoteStore(server.URL, "token", "alice")
			err := remote.commitRemoteTemplate(t.Context(), "Reviewer", "main", files, true, "Reviewer")
			if scenario == "manual_retry" {
				if err == nil || commits != 2 || string(remoteFiles[localManifestFileName]) != "old" {
					t.Fatalf("failed publication advanced the version marker: commits=%d err=%v", commits, err)
				}
				err = remote.commitRemoteTemplate(t.Context(), "Reviewer", "main", files, true, "Reviewer")
			}
			if err != nil {
				t.Fatal(err)
			}
			wantReads, wantCommits := 2, 4
			switch scenario {
			case "manifest_response_lost", "initial_head_unavailable", "initial_tree_unavailable":
				wantCommits = 3
			case "recovery_tree_unavailable":
				wantReads = 3
			}
			if reads != wantReads || commits != wantCommits || !reflect.DeepEqual(remoteFiles, files) {
				t.Fatalf("reads=%d commits=%d files=%v", reads, commits, remoteFiles)
			}
		})
	}
}

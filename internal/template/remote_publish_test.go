package template

import (
	"context"
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
							http.Error(w, "update failed", http.StatusInternalServerError)
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
				case "/api/v1/codes/wanghj/ReviewBot/refs/release/tree/":
					_, _ = w.Write([]byte(`{"data":{"files":[{"path":"obsolete.txt","type":"file"},{"path":".gitattributes","type":"file"}],"cursor":""}}`))
				case "/api/v1/codes/wanghj/ReviewBot/preupload/main", "/api/v1/codes/wanghj/ReviewBot/preupload/release":
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
				case "/api/v1/codes/wanghj/ReviewBot/commit/main", "/api/v1/codes/wanghj/ReviewBot/commit/release":
					if scenario == "commit_error" {
						http.Error(w, "commit failed", 500)
						return
					}
					var request struct {
						Files []remoteCommitFile `json:"files"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					committed = request.Files
					w.WriteHeader(200)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
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
				want := []string{"GET /api/v1/codes/wanghj/ReviewBot", "POST /api/v1/codes", "POST /api/v1/codes/wanghj/ReviewBot/preupload/main", "POST /api/v1/codes/wanghj/ReviewBot/commit/main"}
				if !reflect.DeepEqual(events, want) {
					t.Fatalf("events=%v", events)
				}
			}
			if scenario == "update" {
				want := []string{"GET /api/v1/codes/wanghj/ReviewBot", "PUT /api/v1/codes/wanghj/ReviewBot", "GET /api/v1/codes/wanghj/ReviewBot/refs/release/tree/", "POST /api/v1/codes/wanghj/ReviewBot/preupload/release", "POST /api/v1/codes/wanghj/ReviewBot/commit/release"}
				if !reflect.DeepEqual(events, want) {
					t.Fatalf("events=%v", events)
				}
				if updatedNickname != "Updated Reviewer" {
					t.Fatalf("updated nickname=%q", updatedNickname)
				}
			}
			if scenario == "lookup_error" || scenario == "conflict" {
				if len(events) != 1 {
					t.Fatalf("unexpected mutation: %v", events)
				}
			}
			if scenario == "metadata_error" && len(events) != 2 {
				t.Fatalf("metadata failure continued publication: %v", events)
			}
			if !wantSuccess {
				if len(committed) > 0 {
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
	files := make(map[string]bool)
	if err := remote.remotePublicationFiles(context.Background(), "Reviewer", "main", "", files); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || !reflect.DeepEqual(files, map[string]bool{"agent.toml": true, "skills/old.txt": true}) {
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

func TestRemoteCommitReplacesConflictingFileAndDirectory(t *testing.T) {
	for _, directoryToFile := range []bool{true, false} {
		t.Run(fmt.Sprint(directoryToFile), func(t *testing.T) {
			oldPath, newPath := "instructions/foo/bar", "instructions/foo"
			if !directoryToFile {
				oldPath, newPath = newPath, oldPath
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
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
			if err := remote.commitRemoteTemplate(context.Background(), "Reviewer", "main", map[string][]byte{newPath: []byte("content")}, true); err != nil {
				t.Fatal(err)
			}
		})
	}
}

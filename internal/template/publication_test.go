package template

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	toml "github.com/pelletier/go-toml/v2"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"csgclaw/internal/apitypes"
	"csgclaw/internal/config"
)

func TestPublishLatestUpdatesCloudAndPersistsAssociation(t *testing.T) {
	ctx := context.Background()
	local := NewLocalStore(t.TempDir())
	spec := versionSpec(t, "Reviewer")
	first, err := local.SaveAgent(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	creates, deletes := 0, 0
	exists := false
	activeBranch := "main"
	var uploadedRevisions []int
	var updatedNicknames []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/last_commit"):
			if r.URL.Query().Get("ref") != activeBranch {
				t.Errorf("unexpected last commit branch: %s", r.URL)
			}
			_, _ = w.Write([]byte(`{"data":{"id":"base"}}`))
		case r.URL.Path == "/api/v1/codes/alice/Reviewer/refs/base/tree/":
			_, _ = w.Write([]byte(`{"data":{"files":[]}}`))
		case strings.Contains(r.URL.Path, "/preupload/"):
			respondTemplatePreupload(t, w, r)
		case r.URL.Path == "/api/v1/codes/alice/Reviewer/commit/main":
			var request struct {
				Files []remoteCommitFile `json:"files"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			for _, file := range request.Files {
				if strings.HasPrefix(file.Path, ".versions") {
					t.Errorf("uploaded local history: %s", file.Path)
				}
				if file.Path != "agent.toml" {
					continue
				}
				data, err := base64.StdEncoding.DecodeString(file.Content)
				if err != nil {
					t.Error(err)
					return
				}
				var manifest templateManifest
				if err := toml.Unmarshal(data, &manifest); err != nil {
					t.Error(err)
					return
				}
				if manifest.UID != first.UID || manifest.Version != fmt.Sprintf("V%d", manifest.Revision) {
					t.Errorf("uploaded version/identity mismatch: %+v", manifest)
				}
				uploadedRevisions = append(uploadedRevisions, manifest.Revision)
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/codes":
			creates++
			if exists {
				t.Error("attempted to recreate an existing repository")
				http.Error(w, "duplicate", 500)
				return
			}
			exists = true
			var request remoteCreateCodeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request.Name != "Reviewer" {
				t.Errorf("cloud association lost after rename: %s", request.Name)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"path": "alice/Reviewer"}})
		case r.URL.Path == "/api/v1/codes/alice/Reviewer":
			if r.Method == http.MethodGet {
				if !exists {
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"path": "alice/Reviewer", "default_branch": activeBranch}})
				return
			}
			if r.Method == http.MethodPut {
				var request remoteUpdateCodeRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				updatedNicknames = append(updatedNicknames, request.Nickname)
				w.WriteHeader(http.StatusOK)
				return
			}
			if r.Method != http.MethodDelete || r.Header.Get("Authorization") != "Bearer token" {
				t.Errorf("unexpected deletion request: %s", r.Method)
			}
			deletes++
			exists = false
			_, _ = w.Write([]byte(`{"data":null}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	remote := NewRemoteStore(srv.URL, "token")
	remote.username = "alice"
	svc := mustService(t, config.HubConfig{DefaultRegistry: "local", DefaultPublishRegistry: "local", Registries: []config.HubRegistryConfig{
		{Name: "local", Kind: RegistryKindLocal, Enabled: true}, {Name: "official", Kind: RegistryKindRemote, Enabled: true, URL: srv.URL},
	}}, map[string]Store{"local": local, "official": remote})
	if _, err := svc.PublishTemplate(ctx, "local.Reviewer", "official", false, 0); !errors.Is(err, ErrTemplateVersionChanged) {
		t.Fatalf("stale confirmation error = %v", err)
	}
	if creates != 0 || deletes != 0 || len(uploadedRevisions) != 0 {
		t.Fatal("stale version caused remote side effects")
	}
	for i := 1; i <= 2; i++ {
		if i == 2 {
			spec.DisplayName = "Renamed Reviewer"
			if _, err := local.SaveAgent(ctx, spec); err != nil {
				t.Fatal(err)
			}
		}
		published, err := svc.PublishTemplate(ctx, "local.Reviewer", "official", false, i)
		if err != nil {
			t.Fatal(err)
		}
		if len(uploadedRevisions) != i || uploadedRevisions[i-1] != i {
			t.Fatalf("uploaded revisions = %v", uploadedRevisions)
		}
		if published.UID != first.UID || published.Revision != i {
			t.Fatalf("published = %+v", published)
		}
		current, err := local.Get(ctx, "Reviewer")
		if err != nil {
			t.Fatal(err)
		}
		if len(current.Publications) != 1 || current.Publications[0].Revision != i || current.Publications[0].TemplateID != "alice/Reviewer" {
			t.Fatalf("association = %+v", current.Publications)
		}
	}
	rolled, err := local.RestoreVersion(ctx, first.ID, 1, "", nil)
	if err != nil || rolled.Revision != 1 || rolled.LatestRevision != 2 {
		t.Fatalf("rollback=%+v err=%v", rolled, err)
	}
	if _, err := svc.PublishTemplate(ctx, "local.Reviewer", "official", false, 1); err != nil {
		t.Fatal(err)
	}
	if len(uploadedRevisions) != 3 || uploadedRevisions[2] != 1 {
		t.Fatalf("published wrong active revision: %v", uploadedRevisions)
	}
	current, err := local.Get(ctx, first.ID)
	if err != nil || current.Publications[0].Revision != 1 || current.LatestRevision != 2 {
		t.Fatalf("published current=%+v err=%v", current, err)
	}
	if creates != 1 || deletes != 0 {
		t.Fatalf("creates=%d deletes=%d", creates, deletes)
	}
	if !reflect.DeepEqual(updatedNicknames, []string{"Renamed Reviewer", "Reviewer"}) {
		t.Fatalf("updated nicknames=%v", updatedNicknames)
	}
}

func TestCloudCommitFailureKeepsLocalVersions(t *testing.T) {
	local := NewLocalStore(t.TempDir())
	spec := versionSpec(t, "Reviewer")
	saved, err := local.SaveAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/tree/"):
			_, _ = w.Write([]byte(`{"data":{"files":[]}}`))
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"path":"alice/Reviewer"}}`))
		case strings.Contains(r.URL.Path, "/preupload/"):
			respondTemplatePreupload(t, w, r)
		case strings.Contains(r.URL.Path, "/commit/"):
			http.Error(w, "failed", 500)
		default:
			t.Errorf("unexpected mutation %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	remote := NewAuthenticatedRemoteStore(server.URL, "token", "alice")
	spec.UID = saved.UID
	spec.ReplaceRemote = true
	if _, err := remote.Publish(context.Background(), spec); err == nil {
		t.Fatal("expected commit failure")
	}
	versions, err := local.Versions(context.Background(), saved.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("local history lost: %+v, %v", versions, err)
	}
}

func TestPublicationAssociationDoesNotOverwriteNewIdentity(t *testing.T) {
	local := NewLocalStore(t.TempDir())
	ctx := context.Background()
	spec := versionSpec(t, "Reviewer")
	saved, err := local.SaveAgent(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	publication := apitypes.TemplatePublication{Site: "https://example.invalid", Namespace: "alice", TemplateID: "alice/Reviewer", Revision: 1, PublishedAt: time.Now()}
	if err := local.recordPublication(ctx, saved.ID, "different-identity", publication); err == nil {
		t.Fatal("associated wrong identity")
	}
}

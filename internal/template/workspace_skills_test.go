package template

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"csgclaw/internal/config"
)

func TestRemoteWorkspaceSkillNamesSharesBranchLookupAndBoundsConcurrency(t *testing.T) {
	const endpoint = "/api/v1/codes/agentichub/reviewer"
	const tree = endpoint + "/refs/review-branch/tree/"
	const skillCount = 16
	var branchCalls, directoryCalls, active, peak atomic.Int32
	firstBatch := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == endpoint:
			branchCalls.Add(1)
			_, _ = fmt.Fprint(w, `{"data":{"default_branch":"review-branch"}}`)
		case r.URL.Path == tree+"skills":
			files := make([]map[string]string, 0, skillCount)
			for i := range skillCount {
				name := fmt.Sprintf("skill-%02d", i)
				files = append(files, map[string]string{"name": name, "type": "dir", "path": "skills/" + name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"Files": files}})
		case strings.HasPrefix(r.URL.Path, tree+"skills/"):
			concurrent := active.Add(1)
			defer active.Add(-1)
			for previous := peak.Load(); concurrent > previous && !peak.CompareAndSwap(previous, concurrent); previous = peak.Load() {
			}
			if directoryCalls.Add(1) == workspaceSkillScanConcurrency {
				close(firstBatch)
			}
			select {
			case <-firstBatch:
			case <-r.Context().Done():
				return
			}
			dir := strings.TrimPrefix(r.URL.Path, tree)
			_, _ = fmt.Fprintf(w, `{"data":{"Files":[{"name":"SKILL.md","type":"file","path":%q}]}}`, dir+"/SKILL.md")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc, err := NewService(config.HubConfig{
		DefaultRegistry: "official",
		Registries:      []config.HubRegistryConfig{{Name: "official", Kind: RegistryKindRemote, Enabled: true}},
	}, func(config.HubRegistryConfig) (Store, error) {
		return NewRemoteStore(srv.URL, ""), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	names := svc.WorkspaceSkillNames(ctx, "agentichub/reviewer")
	if len(names) != skillCount || names[0] != "skill-00" || names[len(names)-1] != "skill-15" {
		t.Fatalf("verified skills = %v", names)
	}
	if branchCalls.Load() != 1 || directoryCalls.Load() != skillCount || peak.Load() != workspaceSkillScanConcurrency {
		t.Fatalf("branch calls = %d, directory calls = %d, peak concurrency = %d", branchCalls.Load(), directoryCalls.Load(), peak.Load())
	}
}

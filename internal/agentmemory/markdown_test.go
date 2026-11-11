package agentmemory

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMarkdownRoundTripRetainsSourcesAndIDs(t *testing.T) {
	existing := []Entry{
		{ID: "a", Kind: "preference", Text: "Use Chinese.\nKeep the report short.", Source: &Source{"private", "source"}, UpdatedAt: time.Now()},
		{ID: "b", Kind: "fact", Text: "The project is called example."},
	}
	got, err := parseMarkdown(markdown(existing), existing)
	if err != nil || !reflect.DeepEqual(got, existing) {
		t.Fatalf("round trip = %+v, %v", got, err)
	}
}

func TestMarkdownImportsCategoriesAndUnclassifiedText(t *testing.T) {
	content := "# Agent memory\n\n## 偏好\n- 中文回复\n\n## 工作约定\n- 合并前验证\n\n## 已确认事实\n项目代号 Alpha。\n\n## Project context\nKeep this section and its formatting.\n"
	entries, err := parseMarkdown(content, nil)
	if err != nil || len(entries) != 4 {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	if entries[0].Kind != "preference" || entries[1].Kind != "convention" ||
		entries[2].Kind != "fact" || !strings.Contains(markdown(entries), "Project context") {
		t.Fatalf("category import = %+v", entries)
	}
}

func TestMemoryFailureSurvivesRestartAndSuccessfulRetryClearsIt(t *testing.T) {
	root := t.TempDir()
	store := openTestStore(t, root, "local")
	enableMemory(t, store, testScope)
	before, ok := store.next(testScope, time.Now())
	if !ok {
		t.Fatal("initialization was not queued")
	}
	task := before.Jobs[0]
	if err := store.finish(before, task, nil, nil, errors.New("HTTP 429: rate limited; password=fixture-secret"), nil); err != nil {
		t.Fatal(err)
	}
	store = openTestStore(t, root, "local")
	doc := getMemory(t, store, testScope)
	if doc.LastError == nil || !strings.Contains(doc.LastError.Message, "429") ||
		strings.Contains(doc.LastError.Message, "fixture-secret") || doc.LastError.Operation != "initialization" {
		t.Fatalf("last error = %+v", doc.LastError)
	}
	store.mu.Lock()
	current, err := store.load(testScope)
	if err == nil {
		current.Jobs[0].NextAttempt = time.Now()
		err = store.commit(current)
	}
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	Worker{Store: store}.process(t.Context(), testScope)
	if got := getMemory(t, store, testScope); got.LastError != nil || got.Initialization != "complete" {
		t.Fatalf("successful retry did not clear the error: %+v", got)
	}
}

func TestMemoryUsesReadableIDsAndRejectsUnsafeScopes(t *testing.T) {
	store := openTestStore(t, t.TempDir(), "local")
	enableMemory(t, store, testScope)
	if !strings.HasSuffix(store.scopeDir(testScope), "agent-weekly/memory/user-admin") {
		t.Fatal("scope directory is opaque", store.scopeDir(testScope))
	}
	if _, err := store.Get(Scope{AgentID: "agent-../../outside", UserID: "user-admin"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsafe scope error = %v", err)
	}
	if err := store.DeleteAgent("agent-../../outside"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsafe deletion error = %v", err)
	}
}

func TestMarkdownCodeFenceDoesNotChangeCategories(t *testing.T) {
	original := []Entry{{ID: "code", Kind: "convention", Text: "```markdown\n## 偏好\n- this is code\n```"}}
	got, err := parseMarkdown(markdown(original), original)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatalf("fenced round trip = %+v, %v", got, err)
	}
}

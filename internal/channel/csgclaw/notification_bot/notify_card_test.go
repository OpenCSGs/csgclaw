package notification_bot

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func parseCard(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, s)
	}
	return m
}

func metaValue(t *testing.T, m map[string]any, wantLabel string) string {
	t.Helper()
	meta, ok := m["meta"].([]any)
	if !ok {
		return ""
	}
	for _, row := range meta {
		rm, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if lab, _ := rm["label"].(string); lab == wantLabel {
			v, _ := rm["value"].(string)
			return v
		}
	}
	return ""
}

func TestFormatPayloadAsChatContentGenericJSON(t *testing.T) {
	s := FormatPayloadAsChatContent([]byte(`{"a":1}`), "application/json", nil)
	c := parseCard(t, s)
	if c["type"] != NotifyCardType {
		t.Fatalf("type: %v", c["type"])
	}
	if c["provider"] != NotifyCardProviderGeneric {
		t.Fatalf("provider: %v", c["provider"])
	}
	raw, _ := c["raw"].(string)
	if !strings.Contains(raw, `"a"`) {
		t.Fatalf("raw: %s", raw)
	}
}

func TestFormatPayloadAsChatContentGitLabMergeRequest(t *testing.T) {
	payload := `{
  "object_kind": "merge_request",
  "user": {"name": "Alice", "username": "alice"},
  "project": {"path_with_namespace": "acme/app"},
  "object_attributes": {
    "title": "Fix bug",
    "action": "open",
    "source_branch": "fix",
    "target_branch": "main",
    "url": "https://gitlab.example/acme/app/-/merge_requests/1"
  }
}`
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", nil)
	c := parseCard(t, s)
	if c["provider"] != NotifyCardProviderGitLab || c["event"] != "merge_request" {
		t.Fatalf("%v", c)
	}
	if c["badge"] != "open" || c["subtitle"] != "acme/app" {
		t.Fatalf("%v", c)
	}
	if c["link"] != "https://gitlab.example/acme/app/-/merge_requests/1" {
		t.Fatalf("link: %v", c["link"])
	}
	if metaValue(t, c, "标题") != "Fix bug" {
		t.Fatal(metaValue(t, c, "标题"))
	}
	if metaValue(t, c, "分支") != "fix → main" {
		t.Fatal(metaValue(t, c, "分支"))
	}
	if metaValue(t, c, "操作者") != "Alice (@alice)" {
		t.Fatal(metaValue(t, c, "操作者"))
	}
	if _, has := c["raw"]; has {
		t.Fatal("unexpected raw")
	}
}

func TestFormatPayloadAsChatContentGitLabPush(t *testing.T) {
	payload := `{
  "object_kind": "push",
  "ref": "refs/heads/main",
  "total_commits_count": 2,
  "user": {"name": "Bob", "username": "bob"},
  "project": {"path_with_namespace": "acme/app"},
  "commits": [{"message": "Second line\n\nBody"}, {"message": "Latest commit\nfix"}]
}`
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", nil)
	c := parseCard(t, s)
	if c["event"] != "push" {
		t.Fatal(c)
	}
	if metaValue(t, c, "Ref") != "refs/heads/main" {
		t.Fatal(metaValue(t, c, "Ref"))
	}
	if metaValue(t, c, "提交数") != "2" {
		t.Fatal(metaValue(t, c, "提交数"))
	}
	if !strings.Contains(metaValue(t, c, "最新提交"), "Latest commit") {
		t.Fatal(metaValue(t, c, "最新提交"))
	}
}

func TestFormatPayloadAsChatContentGitLabPipeline(t *testing.T) {
	payload := `{
  "object_kind": "pipeline",
  "project": {"path_with_namespace": "acme/app"},
  "object_attributes": {"status": "success", "ref": "main", "sha": "deadbeefcafe"}
}`
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", nil)
	c := parseCard(t, s)
	if c["badge"] != "success" {
		t.Fatal(c)
	}
	if metaValue(t, c, "SHA") != "deadbeef" {
		t.Fatal(metaValue(t, c, "SHA"))
	}
}

func TestFormatPayloadAsChatContentGitLabNoteIssue(t *testing.T) {
	payload := `{
  "object_kind": "note",
  "event_type": "note",
  "user": {
    "name": "Administrator",
    "username": "root"
  },
  "project": {
    "path_with_namespace": "gitlab-org/gitlab-test"
  },
  "object_attributes": {
    "note": "Hello world",
    "noteable_type": "Issue",
    "noteable_id": 92,
    "action": "create",
    "url": "http://example.com/gitlab-org/gitlab-test/issues/17#note_1241"
  },
  "issue": {
    "iid": 17,
    "title": "test"
  }
}`
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", nil)
	c := parseCard(t, s)
	if c["event"] != "note_issue" {
		t.Fatal(c)
	}
	if !strings.Contains(metaValue(t, c, "Issue"), "#17") || !strings.Contains(metaValue(t, c, "Issue"), "test") {
		t.Fatal(metaValue(t, c, "Issue"))
	}
	if metaValue(t, c, "评论") != "Hello world" {
		t.Fatal(metaValue(t, c, "评论"))
	}
	if !strings.Contains(metaValue(t, c, "评论者"), "Administrator") {
		t.Fatal(metaValue(t, c, "评论者"))
	}
	if c["link"] != "http://example.com/gitlab-org/gitlab-test/issues/17#note_1241" {
		t.Fatal(c["link"])
	}
	if _, has := c["raw"]; has {
		t.Fatal("unexpected raw")
	}
}

func TestFormatPayloadAsChatContentGitLabNoteMergeRequest(t *testing.T) {
	payload := `{
  "object_kind": "note",
  "user": {"name": "Admin", "username": "root"},
  "project": {"path_with_namespace": "gitlab-org/gitlab-test"},
  "object_attributes": {
    "note": "This MR needs work.",
    "noteable_type": "MergeRequest",
    "action": "create",
    "url": "http://example.com/gitlab-org/gitlab-test/merge_requests/1#note_1244"
  },
  "merge_request": {
    "iid": 1,
    "title": "Example MR",
    "source_branch": "master",
    "target_branch": "markdown"
  }
}`
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", nil)
	c := parseCard(t, s)
	if !strings.Contains(metaValue(t, c, "MR"), "!1") || !strings.Contains(metaValue(t, c, "MR"), "Example MR") {
		t.Fatal(metaValue(t, c, "MR"))
	}
	if metaValue(t, c, "分支") != "master → markdown" {
		t.Fatal(metaValue(t, c, "分支"))
	}
	if !strings.Contains(metaValue(t, c, "评论"), "This MR needs work.") {
		t.Fatal(metaValue(t, c, "评论"))
	}
}

func TestFormatPayloadAsChatContentGitHubPullRequest(t *testing.T) {
	payload := `{
  "action": "opened",
  "repository": {"full_name": "acme/app"},
  "pull_request": {
    "title": "Feature",
    "html_url": "https://github.com/acme/app/pull/3",
    "head": {"ref": "feat"},
    "base": {"ref": "main"}
  }
}`
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", nil)
	c := parseCard(t, s)
	if c["provider"] != NotifyCardProviderGitHub || c["event"] != "pull_request" {
		t.Fatal(c)
	}
	if c["badge"] != "opened" || c["subtitle"] != "acme/app" {
		t.Fatal(c)
	}
	if metaValue(t, c, "标题") != "Feature" {
		t.Fatal(metaValue(t, c, "标题"))
	}
	if metaValue(t, c, "分支") != "feat → main" {
		t.Fatal(metaValue(t, c, "分支"))
	}
	if c["link"] != "https://github.com/acme/app/pull/3" {
		t.Fatal(c["link"])
	}
}

func TestFormatPayloadAsChatContentGitHubPush(t *testing.T) {
	payload := `{
  "ref": "refs/heads/main",
  "repository": {"full_name": "acme/app"},
  "pusher": {"name": "Pat"},
  "commits": [{"message": "one"}, {"message": "two\n\nmore"}]
}`
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", nil)
	c := parseCard(t, s)
	if c["event"] != "push" {
		t.Fatal(c)
	}
	if !strings.Contains(metaValue(t, c, "最新提交"), "two") {
		t.Fatal(metaValue(t, c, "最新提交"))
	}
}

func TestFormatPayloadAsChatContentGitHubPing(t *testing.T) {
	payload := `{"zen":"Speak like a human.","repository":{"full_name":"octocat/Hello-World"}}`
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", nil)
	c := parseCard(t, s)
	if c["event"] != "ping" || !strings.Contains(c["summary"].(string), "Speak like a human") {
		t.Fatal(c)
	}
}

func TestFormatPayloadAsChatContentGitHubIssue(t *testing.T) {
	payload := `{
  "action": "opened",
  "repository": {"full_name": "acme/app"},
  "issue": {"title": "Bug", "html_url": "https://github.com/acme/app/issues/9"}
}`
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", nil)
	c := parseCard(t, s)
	if c["event"] != "issue" || metaValue(t, c, "标题") != "Bug" {
		t.Fatal(c)
	}
}

func TestFormatPayloadAsChatContentEmpty(t *testing.T) {
	s := FormatPayloadAsChatContent(nil, "", nil)
	c := parseCard(t, s)
	if c["event"] != "empty" {
		t.Fatal(c)
	}
}

func TestFormatPayloadAsChatContentNonJSON(t *testing.T) {
	s := FormatPayloadAsChatContent([]byte("  hello plain  "), "text/plain", nil)
	c := parseCard(t, s)
	if c["event"] != "text" || c["summary"] != "hello plain" {
		t.Fatalf("%v", c)
	}
}

func TestFormatPayloadAsChatContentGitLabWebhookHeader(t *testing.T) {
	payload := `{
  "object_kind": "merge_request",
  "user": {"name": "Alice", "username": "alice"},
  "project": {"path_with_namespace": "acme/app"},
  "object_attributes": {
    "title": "Fix bug",
    "action": "open",
    "source_branch": "fix",
    "target_branch": "main",
    "url": "https://gitlab.example/acme/app/-/merge_requests/1"
  }
}`
	h := http.Header{}
	h.Set("X-Gitlab-Event", "Merge Request Hook")
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", h)
	c := parseCard(t, s)
	if c["provider"] != NotifyCardProviderGitLab || c["event"] != "merge_request" {
		t.Fatalf("%v", c)
	}
}

func TestFormatPayloadAsChatContentMisleadingGitHubHeaderFallsBackToGitLabBody(t *testing.T) {
	payload := `{
  "object_kind": "push",
  "ref": "refs/heads/main",
  "total_commits_count": 1,
  "user": {"name": "Bob", "username": "bob"},
  "project": {"path_with_namespace": "acme/app"},
  "commits": [{"message": "x"}]
}`
	h := http.Header{}
	h.Set("X-GitHub-Event", "push")
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", h)
	c := parseCard(t, s)
	if c["provider"] != NotifyCardProviderGitLab {
		t.Fatalf("provider %v", c["provider"])
	}
	if c["event"] != "push" {
		t.Fatalf("event %v", c["event"])
	}
}

func TestFormatPayloadAsChatContentGitHubWebhookHeader(t *testing.T) {
	payload := `{
  "action": "opened",
  "repository": {"full_name": "acme/app"},
  "pull_request": {
    "title": "Feature",
    "html_url": "https://github.com/acme/app/pull/3",
    "head": {"ref": "feat"},
    "base": {"ref": "main"}
  }
}`
	h := http.Header{}
	h.Set("X-GitHub-Event", "pull_request")
	s := FormatPayloadAsChatContent([]byte(payload), "application/json", h)
	c := parseCard(t, s)
	if c["provider"] != NotifyCardProviderGitHub || c["event"] != "pull_request" {
		t.Fatal(c)
	}
}

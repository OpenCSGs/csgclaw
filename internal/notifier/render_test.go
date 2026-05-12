package notifier

import (
	"strings"
	"testing"
)

func TestFormatPayloadAsMarkdownJSON(t *testing.T) {
	md := FormatPayloadAsMarkdown([]byte(`{"a":1}`), "application/json")
	if !strings.Contains(md, "```json") {
		t.Fatalf("expected json fence: %s", md)
	}
}

func TestFormatPayloadAsMarkdownGitLabMergeRequest(t *testing.T) {
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
	md := FormatPayloadAsMarkdown([]byte(payload), "application/json")
	for _, want := range []string{
		"## GitLab · Merge request · open",
		"**项目:** acme/app",
		"**标题:** Fix bug",
		"**分支:** `fix` → `main`",
		"https://gitlab.example/acme/app/-/merge_requests/1",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "```json") {
		t.Fatalf("should not fall back to raw json: %s", md)
	}
}

func TestFormatPayloadAsMarkdownGitLabPush(t *testing.T) {
	payload := `{
  "object_kind": "push",
  "ref": "refs/heads/main",
  "total_commits_count": 2,
  "user": {"name": "Bob", "username": "bob"},
  "project": {"path_with_namespace": "acme/app"},
  "commits": [{"message": "Second line\n\nBody"}, {"message": "Latest commit\nfix"}]
}`
	md := FormatPayloadAsMarkdown([]byte(payload), "application/json")
	if !strings.Contains(md, "## GitLab · Push") {
		t.Fatal(md)
	}
	if !strings.Contains(md, "**Ref:** `refs/heads/main`") {
		t.Fatal(md)
	}
	if !strings.Contains(md, "**提交数:** 2") {
		t.Fatal(md)
	}
	if !strings.Contains(md, "Latest commit") {
		t.Fatal(md)
	}
}

func TestFormatPayloadAsMarkdownGitLabPipeline(t *testing.T) {
	payload := `{
  "object_kind": "pipeline",
  "project": {"path_with_namespace": "acme/app"},
  "object_attributes": {"status": "success", "ref": "main", "sha": "deadbeefcafe"}
}`
	md := FormatPayloadAsMarkdown([]byte(payload), "application/json")
	if !strings.Contains(md, "## GitLab · Pipeline · success") {
		t.Fatal(md)
	}
	if !strings.Contains(md, "**SHA:** `deadbeef`") {
		t.Fatal(md)
	}
}

// GitLab Note Hook — comment on an issue (shape from GitLab webhook_events docs).
func TestFormatPayloadAsMarkdownGitLabNoteIssue(t *testing.T) {
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
	md := FormatPayloadAsMarkdown([]byte(payload), "application/json")
	for _, want := range []string{
		"## GitLab · Issue 评论 · create",
		"**项目:** gitlab-org/gitlab-test",
		"**Issue:** #17 test",
		"**评论:** Hello world",
		"Administrator (@root)",
		"http://example.com/gitlab-org/gitlab-test/issues/17#note_1241",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "```json") {
		t.Fatalf("should not fall back to raw json: %s", md)
	}
}

func TestFormatPayloadAsMarkdownGitLabNoteMergeRequest(t *testing.T) {
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
	md := FormatPayloadAsMarkdown([]byte(payload), "application/json")
	for _, want := range []string{
		"## GitLab · MR 评论 · create",
		"!1 Example MR",
		"`master` → `markdown`",
		"This MR needs work.",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in:\n%s", want, md)
		}
	}
}

func TestFormatPayloadAsMarkdownGitHubPullRequest(t *testing.T) {
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
	md := FormatPayloadAsMarkdown([]byte(payload), "application/json")
	for _, want := range []string{
		"## GitHub · Pull request · opened",
		"acme/app",
		"Feature",
		"`feat` → `main`",
		"https://github.com/acme/app/pull/3",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in:\n%s", want, md)
		}
	}
}

func TestFormatPayloadAsMarkdownGitHubPush(t *testing.T) {
	payload := `{
  "ref": "refs/heads/main",
  "repository": {"full_name": "acme/app"},
  "pusher": {"name": "Pat"},
  "commits": [{"message": "one"}, {"message": "two\n\nmore"}]
}`
	md := FormatPayloadAsMarkdown([]byte(payload), "application/json")
	if !strings.Contains(md, "## GitHub · Push") || !strings.Contains(md, "two") {
		t.Fatal(md)
	}
}

func TestFormatPayloadAsMarkdownGitHubPing(t *testing.T) {
	payload := `{"zen":"Speak like a human.","repository":{"full_name":"octocat/Hello-World"}}`
	md := FormatPayloadAsMarkdown([]byte(payload), "application/json")
	if !strings.Contains(md, "## GitHub · Ping") || !strings.Contains(md, "Speak like a human") {
		t.Fatal(md)
	}
}

func TestFormatPayloadAsMarkdownGitHubIssue(t *testing.T) {
	payload := `{
  "action": "opened",
  "repository": {"full_name": "acme/app"},
  "issue": {"title": "Bug", "html_url": "https://github.com/acme/app/issues/9"}
}`
	md := FormatPayloadAsMarkdown([]byte(payload), "application/json")
	if !strings.Contains(md, "## GitHub · Issue · opened") || !strings.Contains(md, "Bug") {
		t.Fatal(md)
	}
}

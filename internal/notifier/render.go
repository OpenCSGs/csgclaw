package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const maxPayloadMarkdownRunes = 12000

// FormatPayloadAsMarkdown turns webhook bytes into chat Markdown.
// Known GitLab / GitHub webhook shapes get a short summary; others fall back to pretty-printed JSON.
func FormatPayloadAsMarkdown(payload []byte, contentType string) string {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return "_Empty notification body_"
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	jsonLike := strings.Contains(ct, "json") || json.Valid(payload)
	if !jsonLike {
		return truncateMarkdown(fence(string(payload)))
	}
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		return truncateMarkdown(fence(string(payload)))
	}
	if s, ok := renderKnownWebhook(root); ok {
		return truncateMarkdown(s)
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return truncateMarkdown(fence(string(payload)))
	}
	return truncateMarkdown("## Notification\n\n```json\n" + string(out) + "\n```")
}

func renderKnownWebhook(root map[string]any) (string, bool) {
	if kind := strings.TrimSpace(toStr(root["object_kind"])); kind != "" {
		return renderGitLab(kind, root)
	}
	// GitHub & generic JSON APIs
	if _, ok := root["zen"]; ok {
		return renderGitHubPing(root)
	}
	if hasMap(root, "pull_request") && root["action"] != nil {
		return renderGitHubPullRequest(root)
	}
	if hasMap(root, "issue") && root["action"] != nil && root["repository"] != nil {
		return renderGitHubIssue(root)
	}
	if root["commits"] != nil && root["ref"] != nil && root["repository"] != nil {
		return renderGitHubPush(root)
	}
	return "", false
}

func renderGitLab(kind string, root map[string]any) (string, bool) {
	switch strings.ToLower(kind) {
	case "merge_request":
		return renderGitLabMergeRequest(root)
	case "push":
		return renderGitLabPush(root)
	case "pipeline":
		return renderGitLabPipeline(root)
	case "issue":
		return renderGitLabIssue(root)
	case "note":
		return renderGitLabNote(root)
	default:
		return "", false
	}
}

// renderGitLabNote handles GitLab Note Hook (comments on issue, merge request, commit, snippet, etc.).
// See https://docs.gitlab.com/user/project/integrations/webhook_events/#comment-on-an-issue
func renderGitLabNote(root map[string]any) (string, bool) {
	oa := nestedMap(root, "object_attributes")
	noteable := getStr(oa, "noteable_type")
	switch noteable {
	case "Issue":
		if nestedMap(root, "issue") == nil {
			return "", false
		}
		return renderGitLabIssueComment(root)
	case "MergeRequest":
		if nestedMap(root, "merge_request") == nil {
			return "", false
		}
		return renderGitLabMergeRequestComment(root)
	default:
		return "", false
	}
}

func renderGitLabIssueComment(root map[string]any) (string, bool) {
	oa := nestedMap(root, "object_attributes")
	action := getStr(oa, "action")
	if action == "" {
		action = "comment"
	}
	noteText := getStr(oa, "note")
	url := getStr(oa, "url")
	issue := nestedMap(root, "issue")
	title := getStr(issue, "title")
	iid := getStr(issue, "iid")
	proj := nestedMap(root, "project")
	path := getStr(proj, "path_with_namespace")
	if path == "" {
		path = getStr(proj, "name")
	}
	who := formatGitLabUser(nestedMap(root, "user"))

	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab · Issue 评论 · %s\n\n", esc(action))
	if path != "" {
		fmt.Fprintf(&b, "**项目:** %s\n\n", esc(path))
	}
	if title != "" {
		if iid != "" {
			fmt.Fprintf(&b, "**Issue:** #%s %s\n\n", esc(iid), esc(title))
		} else {
			fmt.Fprintf(&b, "**Issue:** %s\n\n", esc(title))
		}
	}
	if noteText != "" {
		fmt.Fprintf(&b, "**评论:** %s\n\n", esc(truncateRunes(noteText, 500)))
	}
	if who != "" {
		fmt.Fprintf(&b, "**评论者:** %s\n\n", esc(who))
	}
	if url != "" {
		fmt.Fprintf(&b, "**链接:** %s\n", url)
	}
	return b.String(), true
}

func renderGitLabMergeRequestComment(root map[string]any) (string, bool) {
	oa := nestedMap(root, "object_attributes")
	action := getStr(oa, "action")
	if action == "" {
		action = "comment"
	}
	noteText := getStr(oa, "note")
	url := getStr(oa, "url")
	mr := nestedMap(root, "merge_request")
	title := getStr(mr, "title")
	iid := getStr(mr, "iid")
	source := getStr(mr, "source_branch")
	target := getStr(mr, "target_branch")
	proj := nestedMap(root, "project")
	path := getStr(proj, "path_with_namespace")
	if path == "" {
		path = getStr(proj, "name")
	}
	who := formatGitLabUser(nestedMap(root, "user"))

	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab · MR 评论 · %s\n\n", esc(action))
	if path != "" {
		fmt.Fprintf(&b, "**项目:** %s\n\n", esc(path))
	}
	if title != "" {
		if iid != "" {
			fmt.Fprintf(&b, "**MR:** !%s %s\n\n", esc(iid), esc(title))
		} else {
			fmt.Fprintf(&b, "**MR:** %s\n\n", esc(title))
		}
	}
	if source != "" || target != "" {
		fmt.Fprintf(&b, "**分支:** `%s` → `%s`\n\n", inBackticks(source), inBackticks(target))
	}
	if noteText != "" {
		fmt.Fprintf(&b, "**评论:** %s\n\n", esc(truncateRunes(noteText, 500)))
	}
	if who != "" {
		fmt.Fprintf(&b, "**评论者:** %s\n\n", esc(who))
	}
	if url != "" {
		fmt.Fprintf(&b, "**链接:** %s\n", url)
	}
	return b.String(), true
}

func formatGitLabUser(user map[string]any) string {
	if user == nil {
		return ""
	}
	name := getStr(user, "name")
	if handle := getStr(user, "username"); handle != "" {
		if name != "" {
			return fmt.Sprintf("%s (@%s)", name, handle)
		}
		return "@" + handle
	}
	return name
}

func renderGitLabMergeRequest(root map[string]any) (string, bool) {
	proj := nestedMap(root, "project")
	title := getStr(nestedMap(root, "object_attributes"), "title")
	if title == "" {
		title = "(no title)"
	}
	action := getStr(nestedMap(root, "object_attributes"), "action")
	if action == "" {
		action = "update"
	}
	source := getStr(nestedMap(root, "object_attributes"), "source_branch")
	target := getStr(nestedMap(root, "object_attributes"), "target_branch")
	url := getStr(nestedMap(root, "object_attributes"), "url")
	path := getStr(proj, "path_with_namespace")
	if path == "" {
		path = getStr(proj, "name")
	}
	user := nestedMap(root, "user")
	who := getStr(user, "name")
	if handle := getStr(user, "username"); handle != "" {
		if who != "" {
			who = fmt.Sprintf("%s (@%s)", who, handle)
		} else {
			who = "@" + handle
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab · Merge request · %s\n\n", esc(action))
	if path != "" {
		fmt.Fprintf(&b, "**项目:** %s\n\n", esc(path))
	}
	fmt.Fprintf(&b, "**标题:** %s\n\n", esc(title))
	if source != "" || target != "" {
		fmt.Fprintf(&b, "**分支:** `%s` → `%s`\n\n", inBackticks(source), inBackticks(target))
	}
	if who != "" {
		fmt.Fprintf(&b, "**操作者:** %s\n\n", esc(who))
	}
	if url != "" {
		fmt.Fprintf(&b, "**链接:** %s\n", url)
	}
	return b.String(), true
}

func renderGitLabPush(root map[string]any) (string, bool) {
	proj := nestedMap(root, "project")
	path := getStr(proj, "path_with_namespace")
	if path == "" {
		path = getStr(proj, "name")
	}
	ref := getStr(root, "ref")
	userName := getStr(nestedMap(root, "user"), "name")
	userHandle := getStr(nestedMap(root, "user"), "username")
	who := userName
	if userHandle != "" {
		if who != "" {
			who = fmt.Sprintf("%s (@%s)", who, userHandle)
		} else {
			who = "@" + userHandle
		}
	}
	nCommits := 0
	if v, ok := root["total_commits_count"].(float64); ok {
		nCommits = int(v)
	}
	lastMsg := ""
	if commits, ok := root["commits"].([]any); ok && len(commits) > 0 {
		if last, ok := commits[len(commits)-1].(map[string]any); ok {
			lastMsg = getStr(last, "message")
			if idx := strings.IndexByte(lastMsg, '\n'); idx >= 0 {
				lastMsg = strings.TrimSpace(lastMsg[:idx])
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab · Push\n\n")
	if path != "" {
		fmt.Fprintf(&b, "**项目:** %s\n\n", esc(path))
	}
	if ref != "" {
		fmt.Fprintf(&b, "**Ref:** `%s`\n\n", inBackticks(ref))
	}
	if who != "" {
		fmt.Fprintf(&b, "**推送者:** %s\n\n", esc(who))
	}
	if nCommits > 0 {
		fmt.Fprintf(&b, "**提交数:** %d\n\n", nCommits)
	}
	if lastMsg != "" {
		fmt.Fprintf(&b, "**最新提交:** %s\n", esc(truncateRunes(lastMsg, 200)))
	}
	return b.String(), true
}

func renderGitLabPipeline(root map[string]any) (string, bool) {
	oa := nestedMap(root, "object_attributes")
	status := getStr(oa, "status")
	ref := getStr(oa, "ref")
	sha := getStr(oa, "sha")
	if len(sha) > 8 {
		sha = sha[:8]
	}
	proj := nestedMap(root, "project")
	path := getStr(proj, "path_with_namespace")
	if path == "" {
		path = getStr(proj, "name")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab · Pipeline · %s\n\n", esc(strings.TrimSpace(status)))
	if path != "" {
		fmt.Fprintf(&b, "**项目:** %s\n\n", esc(path))
	}
	if ref != "" {
		fmt.Fprintf(&b, "**Ref:** `%s`\n\n", inBackticks(ref))
	}
	if sha != "" {
		fmt.Fprintf(&b, "**SHA:** `%s`\n", inBackticks(sha))
	}
	return b.String(), true
}

func renderGitLabIssue(root map[string]any) (string, bool) {
	oa := nestedMap(root, "object_attributes")
	title := getStr(oa, "title")
	action := getStr(oa, "action")
	if action == "" {
		action = "update"
	}
	url := getStr(oa, "url")
	proj := nestedMap(root, "project")
	path := getStr(proj, "path_with_namespace")
	var b strings.Builder
	fmt.Fprintf(&b, "## GitLab · Issue · %s\n\n", esc(action))
	if path != "" {
		fmt.Fprintf(&b, "**项目:** %s\n\n", esc(path))
	}
	if title != "" {
		fmt.Fprintf(&b, "**标题:** %s\n\n", esc(title))
	}
	if url != "" {
		fmt.Fprintf(&b, "**链接:** %s\n", url)
	}
	return b.String(), true
}

func renderGitHubPing(root map[string]any) (string, bool) {
	zen := getStr(root, "zen")
	repo := nestedMap(root, "repository")
	name := getStr(repo, "full_name")
	var b strings.Builder
	fmt.Fprintf(&b, "## GitHub · Ping\n\n")
	if name != "" {
		fmt.Fprintf(&b, "**仓库:** %s\n\n", esc(name))
	}
	if zen != "" {
		fmt.Fprintf(&b, "%s\n", esc(zen))
	}
	return b.String(), true
}

func renderGitHubPullRequest(root map[string]any) (string, bool) {
	action := getStr(root, "action")
	pr := nestedMap(root, "pull_request")
	title := getStr(pr, "title")
	htmlURL := getStr(pr, "html_url")
	head := getStr(nestedMap(pr, "head"), "ref")
	base := getStr(nestedMap(pr, "base"), "ref")
	repo := nestedMap(root, "repository")
	full := getStr(repo, "full_name")
	var b strings.Builder
	fmt.Fprintf(&b, "## GitHub · Pull request · %s\n\n", esc(action))
	if full != "" {
		fmt.Fprintf(&b, "**仓库:** %s\n\n", esc(full))
	}
	if title != "" {
		fmt.Fprintf(&b, "**标题:** %s\n\n", esc(title))
	}
	if head != "" || base != "" {
		fmt.Fprintf(&b, "**分支:** `%s` → `%s`\n\n", inBackticks(head), inBackticks(base))
	}
	if htmlURL != "" {
		fmt.Fprintf(&b, "**链接:** %s\n", htmlURL)
	}
	return b.String(), true
}

func renderGitHubIssue(root map[string]any) (string, bool) {
	action := getStr(root, "action")
	iss := nestedMap(root, "issue")
	title := getStr(iss, "title")
	htmlURL := getStr(iss, "html_url")
	repo := nestedMap(root, "repository")
	full := getStr(repo, "full_name")
	var b strings.Builder
	fmt.Fprintf(&b, "## GitHub · Issue · %s\n\n", esc(action))
	if full != "" {
		fmt.Fprintf(&b, "**仓库:** %s\n\n", esc(full))
	}
	if title != "" {
		fmt.Fprintf(&b, "**标题:** %s\n\n", esc(title))
	}
	if htmlURL != "" {
		fmt.Fprintf(&b, "**链接:** %s\n", htmlURL)
	}
	return b.String(), true
}

func renderGitHubPush(root map[string]any) (string, bool) {
	ref := getStr(root, "ref")
	repo := nestedMap(root, "repository")
	full := getStr(repo, "full_name")
	pusher := nestedMap(root, "pusher")
	who := getStr(pusher, "name")
	var lastMsg string
	if commits, ok := root["commits"].([]any); ok && len(commits) > 0 {
		if last, ok := commits[len(commits)-1].(map[string]any); ok {
			lastMsg = getStr(last, "message")
			if idx := strings.IndexByte(lastMsg, '\n'); idx >= 0 {
				lastMsg = strings.TrimSpace(lastMsg[:idx])
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## GitHub · Push\n\n")
	if full != "" {
		fmt.Fprintf(&b, "**仓库:** %s\n\n", esc(full))
	}
	if ref != "" {
		fmt.Fprintf(&b, "**Ref:** `%s`\n\n", inBackticks(ref))
	}
	if who != "" {
		fmt.Fprintf(&b, "**推送者:** %s\n\n", esc(who))
	}
	if lastMsg != "" {
		fmt.Fprintf(&b, "**最新提交:** %s\n", esc(truncateRunes(lastMsg, 200)))
	}
	return b.String(), true
}

func nestedMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	mm, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return mm
}

func hasMap(m map[string]any, key string) bool {
	return nestedMap(m, key) != nil
}

func getStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(toStr(v))
}

func toStr(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%.0f", t)
		}
		return fmt.Sprint(t)
	case bool:
		return fmt.Sprint(t)
	case json.Number:
		return t.String()
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func inBackticks(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}

func esc(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "*", "\\*")
	s = strings.ReplaceAll(s, "_", "\\_")
	s = strings.ReplaceAll(s, "`", "\\`")
	return s
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func fence(s string) string {
	s = strings.ReplaceAll(s, "```", "`\u200b``")
	return "```\n" + s + "\n```"
}

func truncateMarkdown(s string) string {
	r := []rune(s)
	if len(r) <= maxPayloadMarkdownRunes {
		return s
	}
	head := string(r[:maxPayloadMarkdownRunes])
	return head + "\n\n_(truncated)_"
}

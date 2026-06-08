package notification

import (
	"encoding/json"
	"fmt"
	"strings"
)

// cardFromGitLabWebhookBody parses GitLab-style webhook JSON (object_kind + payload shape).
func cardFromGitLabWebhookBody(root map[string]any) (NotifyCard, bool) {
	if kind := strings.TrimSpace(toStr(root["object_kind"])); kind != "" {
		return cardGitLab(strings.ToLower(kind), root)
	}
	return NotifyCard{}, false
}

// cardFromGitHubWebhookBody parses GitHub-style webhook JSON (zen, pull_request, issue, push shapes).
func cardFromGitHubWebhookBody(root map[string]any) (NotifyCard, bool) {
	if _, ok := root["zen"]; ok {
		return cardGitHubPing(root), true
	}
	if hasMap(root, "pull_request") && root["action"] != nil {
		return cardGitHubPullRequest(root), true
	}
	if hasMap(root, "issue") && root["action"] != nil && root["repository"] != nil {
		return cardGitHubIssue(root), true
	}
	if root["commits"] != nil && root["ref"] != nil && root["repository"] != nil {
		return cardGitHubPush(root), true
	}
	return NotifyCard{}, false
}

func cardFromKnownWebhooks(root map[string]any) (NotifyCard, bool) {
	if card, ok := cardFromGitLabWebhookBody(root); ok {
		return card, true
	}
	return cardFromGitHubWebhookBody(root)
}

func cardGitLab(kind string, root map[string]any) (NotifyCard, bool) {
	switch kind {
	case "merge_request":
		return cardGitLabMergeRequest(root)
	case "push":
		return cardGitLabPush(root), true
	case "pipeline":
		return cardGitLabPipeline(root), true
	case "issue":
		return cardGitLabIssue(root), true
	case "note":
		return cardGitLabNote(root)
	default:
		return NotifyCard{}, false
	}
}

func cardGitLabNote(root map[string]any) (NotifyCard, bool) {
	oa := nestedMap(root, "object_attributes")
	noteable := getStr(oa, "noteable_type")
	switch noteable {
	case "Issue":
		if nestedMap(root, "issue") == nil {
			return NotifyCard{}, false
		}
		return cardGitLabIssueComment(root), true
	case "MergeRequest":
		if nestedMap(root, "merge_request") == nil {
			return NotifyCard{}, false
		}
		return cardGitLabMergeRequestComment(root), true
	default:
		return NotifyCard{}, false
	}
}

func cardGitLabIssueComment(root map[string]any) NotifyCard {
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

	c := NotifyCard{
		Provider: NotifyCardProviderGitLab,
		Event:    "note_issue",
		Title:    "GitLab · Issue 评论",
		Badge:    action,
		Subtitle: path,
		Link:     url,
	}
	if title != "" {
		if iid != "" {
			c.Meta = append(c.Meta, NotifyMeta{Label: "Issue", Value: "#" + iid + " " + title})
		} else {
			c.Meta = append(c.Meta, NotifyMeta{Label: "Issue", Value: title})
		}
	}
	if noteText != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "评论", Value: truncateRunes(noteText, 500)})
	}
	if who != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "评论者", Value: who})
	}
	return c
}

func cardGitLabMergeRequestComment(root map[string]any) NotifyCard {
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

	c := NotifyCard{
		Provider: NotifyCardProviderGitLab,
		Event:    "note_merge_request",
		Title:    "GitLab · MR 评论",
		Badge:    action,
		Subtitle: path,
		Link:     url,
	}
	if title != "" {
		if iid != "" {
			c.Meta = append(c.Meta, NotifyMeta{Label: "MR", Value: "!" + iid + " " + title})
		} else {
			c.Meta = append(c.Meta, NotifyMeta{Label: "MR", Value: title})
		}
	}
	if source != "" || target != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "分支", Value: branchArrow(source, target)})
	}
	if noteText != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "评论", Value: truncateRunes(noteText, 500)})
	}
	if who != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "评论者", Value: who})
	}
	return c
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

func cardGitLabMergeRequest(root map[string]any) (NotifyCard, bool) {
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
	who := formatGitLabUser(user)

	c := NotifyCard{
		Provider: NotifyCardProviderGitLab,
		Event:    "merge_request",
		Title:    "GitLab · Merge request",
		Badge:    action,
		Subtitle: path,
		Link:     url,
	}
	c.Meta = append(c.Meta, NotifyMeta{Label: "标题", Value: title})
	if source != "" || target != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "分支", Value: branchArrow(source, target)})
	}
	if who != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "操作者", Value: who})
	}
	return c, true
}

func cardGitLabPush(root map[string]any) NotifyCard {
	proj := nestedMap(root, "project")
	path := getStr(proj, "path_with_namespace")
	if path == "" {
		path = getStr(proj, "name")
	}
	ref := getStr(root, "ref")
	who := formatGitLabUser(nestedMap(root, "user"))
	nCommits := gitLabCommitCount(root)
	lastMsg := gitLabLastCommitSubject(root)

	c := NotifyCard{
		Provider: NotifyCardProviderGitLab,
		Event:    "push",
		Title:    "GitLab · Push",
		Subtitle: path,
	}
	if ref != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "Ref", Value: ref})
	}
	if who != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "推送者", Value: who})
	}
	if nCommits > 0 {
		c.Meta = append(c.Meta, NotifyMeta{Label: "提交数", Value: fmt.Sprintf("%d", nCommits)})
	}
	if lastMsg != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "最新提交", Value: truncateRunes(lastMsg, 200)})
	}
	return c
}

func gitLabCommitCount(root map[string]any) int {
	if v, ok := root["total_commits_count"].(float64); ok {
		return int(v)
	}
	return 0
}

func gitLabLastCommitSubject(root map[string]any) string {
	commits, ok := root["commits"].([]any)
	if !ok || len(commits) == 0 {
		return ""
	}
	last, ok := commits[len(commits)-1].(map[string]any)
	if !ok {
		return ""
	}
	lastMsg := getStr(last, "message")
	if idx := strings.IndexByte(lastMsg, '\n'); idx >= 0 {
		lastMsg = strings.TrimSpace(lastMsg[:idx])
	}
	return lastMsg
}

func cardGitLabPipeline(root map[string]any) NotifyCard {
	oa := nestedMap(root, "object_attributes")
	status := strings.TrimSpace(getStr(oa, "status"))
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
	c := NotifyCard{
		Provider: NotifyCardProviderGitLab,
		Event:    "pipeline",
		Title:    "GitLab · Pipeline",
		Badge:    status,
		Subtitle: path,
	}
	if ref != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "Ref", Value: ref})
	}
	if sha != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "SHA", Value: sha})
	}
	return c
}

func cardGitLabIssue(root map[string]any) NotifyCard {
	oa := nestedMap(root, "object_attributes")
	title := getStr(oa, "title")
	action := getStr(oa, "action")
	if action == "" {
		action = "update"
	}
	url := getStr(oa, "url")
	proj := nestedMap(root, "project")
	path := getStr(proj, "path_with_namespace")
	c := NotifyCard{
		Provider: NotifyCardProviderGitLab,
		Event:    "issue",
		Title:    "GitLab · Issue",
		Badge:    action,
		Subtitle: path,
		Link:     url,
	}
	if title != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "标题", Value: title})
	}
	return c
}

func cardGitHubPing(root map[string]any) NotifyCard {
	zen := getStr(root, "zen")
	repo := nestedMap(root, "repository")
	name := getStr(repo, "full_name")
	return NotifyCard{
		Provider: NotifyCardProviderGitHub,
		Event:    "ping",
		Title:    "GitHub · Ping",
		Subtitle: name,
		Summary:  zen,
	}
}

func cardGitHubPullRequest(root map[string]any) NotifyCard {
	action := getStr(root, "action")
	pr := nestedMap(root, "pull_request")
	title := getStr(pr, "title")
	htmlURL := getStr(pr, "html_url")
	head := getStr(nestedMap(pr, "head"), "ref")
	base := getStr(nestedMap(pr, "base"), "ref")
	repo := nestedMap(root, "repository")
	full := getStr(repo, "full_name")
	c := NotifyCard{
		Provider: NotifyCardProviderGitHub,
		Event:    "pull_request",
		Title:    "GitHub · Pull request",
		Badge:    action,
		Subtitle: full,
		Link:     htmlURL,
	}
	if title != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "标题", Value: title})
	}
	if head != "" || base != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "分支", Value: branchArrow(head, base)})
	}
	return c
}

func cardGitHubIssue(root map[string]any) NotifyCard {
	action := getStr(root, "action")
	iss := nestedMap(root, "issue")
	title := getStr(iss, "title")
	htmlURL := getStr(iss, "html_url")
	repo := nestedMap(root, "repository")
	full := getStr(repo, "full_name")
	c := NotifyCard{
		Provider: NotifyCardProviderGitHub,
		Event:    "issue",
		Title:    "GitHub · Issue",
		Badge:    action,
		Subtitle: full,
		Link:     htmlURL,
	}
	if title != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "标题", Value: title})
	}
	return c
}

func cardGitHubPush(root map[string]any) NotifyCard {
	ref := getStr(root, "ref")
	repo := nestedMap(root, "repository")
	full := getStr(repo, "full_name")
	pusher := nestedMap(root, "pusher")
	who := getStr(pusher, "name")
	lastMsg := gitHubLastCommitSubject(root)
	c := NotifyCard{
		Provider: NotifyCardProviderGitHub,
		Event:    "push",
		Title:    "GitHub · Push",
		Subtitle: full,
	}
	if ref != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "Ref", Value: ref})
	}
	if who != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "推送者", Value: who})
	}
	if lastMsg != "" {
		c.Meta = append(c.Meta, NotifyMeta{Label: "最新提交", Value: truncateRunes(lastMsg, 200)})
	}
	return c
}

func gitHubLastCommitSubject(root map[string]any) string {
	commits, ok := root["commits"].([]any)
	if !ok || len(commits) == 0 {
		return ""
	}
	last, ok := commits[len(commits)-1].(map[string]any)
	if !ok {
		return ""
	}
	lastMsg := getStr(last, "message")
	if idx := strings.IndexByte(lastMsg, '\n'); idx >= 0 {
		lastMsg = strings.TrimSpace(lastMsg[:idx])
	}
	return lastMsg
}

func branchArrow(from, to string) string {
	from = strings.ReplaceAll(from, "`", "'")
	to = strings.ReplaceAll(to, "`", "'")
	return from + " → " + to
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

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

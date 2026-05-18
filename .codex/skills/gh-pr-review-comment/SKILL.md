---
name: gh-pr-review-comment
description: Review a GitHub pull request by repo and PR number, inspect the PR diff, draft severity-ranked review findings, and optionally publish selected comments back to the PR after explicit user confirmation. Use when Codex is asked to review a specific PR, prepare actionable review feedback, prioritize only the highest-value comments for publication, or handle a two-phase review flow with a preview before posting PR comments.
---

# GitHub PR Review Comment

Review the requested PR in two phases. First, inspect the diff and produce a concise review summary plus a structured candidate comment list. Second, post comments to the PR only after the user explicitly confirms.

Treat any user-provided PR summary or review direction as guidance, not as truth. Use it to focus the review, but still inspect the actual diff and verify claims against the code.

## Inputs

Collect these inputs before reviewing:

- Repository identifier or local repo path.
- PR number.
- Optional PR summary from the user.
- Optional review direction, such as correctness, regressions, API design, tests, security, performance, readability, or release risk.

If the user omits summary or review direction, continue with a general review.

## Review Workflow

1. Resolve how to access the PR.
2. Fetch the PR diff, changed files, and surrounding context for touched lines.
3. Review the diff with the user's requested focus areas first, then do a general pass for correctness and regression risk.
4. Produce a review summary and a candidate comment list grouped by severity.
5. Ask for confirmation before publishing any PR comments.
6. After confirmation, publish only the selected comments and report what was posted.

Prefer `gh` as the primary execution path. Use GitHub MCP only as a fallback when the CLI is unavailable. When line-specific comments will be posted, confirm the CLI path can target exact files and lines in the current PR revision.

If `gh` is installed and configured but authentication fails only in the current non-interactive Codex session, treat that as a session access problem first, not as a missing token by default. This often means the user authenticated with `gh auth login`, but the sandboxed session cannot read the system keychain or credential store.

## GitHub CLI Workflow

Assume `gh` is the primary tool unless the user says otherwise.

Use the helper script in [scripts/gh_pr_review_helper.py](scripts/gh_pr_review_helper.py) for the mechanical `gh` steps. Keep review judgment in the model, and use the script only for collection, validation, dry-run reporting, and confirmed posting.

Use these commands as the default information sources:

```bash
gh pr view <pr> --repo <owner/repo> \
  --json number,title,body,files,commits,headRefOid,baseRefName,headRefName,url

gh pr diff <pr> --repo <owner/repo> --patch
```

Use `gh pr view --json files` to inspect changed files and changed line ranges. Use `gh pr diff --patch` for the actual review pass and nearby hunk context.

If the user gives only a local repo path and PR number, resolve the repository from the current checkout and still prefer `gh`.

Before doing anything else that mutates GitHub state, verify authentication:

```bash
gh auth status
```

If authentication is missing or insufficient for review comments, stop and tell the user exactly what is blocked.

On macOS, treat a sandbox failure from `gh auth status` as potentially untrustworthy even if the message is vague. Many `gh auth login` setups store credentials in Keychain, and the current non-interactive Codex session may fail to read them cleanly.

If `gh auth status` fails on macOS, do this before falling back:

1. Tell the user that the sandboxed Codex session could not validate `gh` authentication and that this may be a Keychain access issue.
2. Request permission to rerun the `gh` command outside the sandbox with escalated privileges.
3. Retry `gh auth status` once after approval.
4. If the escalated retry succeeds, continue with the `gh` workflow.
5. Only conclude that authentication is actually missing or invalid if the escalated retry still fails, or if the user declines escalation.

On non-macOS systems, or when there is clear evidence that `gh` is not configured at all, use the normal failure path without this extra retry.

Do not infer authentication failure only from the absence of a plaintext token in `~/.config/gh/hosts.yml`. A valid `gh auth login` setup may store credentials in macOS Keychain or another credential helper.

## Helper Script

The helper script has two subcommands:

```bash
python3 scripts/gh_pr_review_helper.py collect --repo <owner/repo> --pr <pr> --output pr.json
python3 scripts/gh_pr_review_helper.py post --input comments.json
python3 scripts/gh_pr_review_helper.py post --input comments.json --confirm
```

Use `collect` to:

- verify `gh` authentication
- assume the caller already followed the macOS escalated-retry rule above when sandbox auth is ambiguous
- distinguish missing `gh` auth from sandboxed credential-store access failures
- resolve the repo when omitted
- fetch PR metadata
- fetch the full patch text
- emit a single JSON payload for the review pass

Use `post` to:

- read a prepared comment payload
- reject posting when the PR head SHA changed since preview
- emit a dry-run report by default
- publish inline comments only when `--confirm` is present

Do not use the helper script to decide whether a finding is valid or what severity it should have.

## Review Focus

Check these areas unless the user narrows scope:

- Behavioral correctness.
- Regressions introduced by the new logic.
- Missing or insufficient tests for changed behavior.
- API and contract compatibility.
- Error handling and edge cases.
- Security and data exposure.
- Performance issues that are plausible from the diff.
- Maintainability problems that materially increase future risk.

Do not manufacture stylistic nits just to increase comment count. Prefer a small number of high-signal findings.

## Severity Model

Use the rubric in [references/review-rubric.md](references/review-rubric.md) when classification is unclear. Apply these levels:

- `critical`: Likely production breakage, data loss, security issue, or a clearly invalid implementation that should block merge.
- `high`: Strong regression risk or correctness issue with meaningful impact.
- `medium`: Real issue worth fixing, but lower impact or narrower scope.
- `low`: Minor improvement, clarity issue, or non-blocking suggestion.

Only publish PR comments by default for `critical` and `high` findings. Include `medium` comments in the preview list for user selection. Keep `low` findings in the written summary unless the user explicitly wants exhaustive comments.

## Drafting Rules

Every candidate comment must be:

- Specific to the changed code.
- Actionable and easy to verify.
- Backed by the diff or nearby code context.
- Written in neutral, professional language.
- Short enough to work as a PR comment without extra cleanup.

Each comment should ideally contain:

- What looks wrong.
- Why it matters.
- What change or check would resolve it.

Avoid overstating certainty. If a finding depends on an assumption, say so plainly.

## Preview Format

Before posting, always show the user:

1. A short overall review summary.
2. A severity breakdown, such as `critical: 1, high: 2, medium: 3, low: 1`.
3. A candidate comment list in a structured format.

Use this table or a close equivalent:

```md
| Select | Severity | File | Line | Comment |
| --- | --- | --- | --- | --- |
| yes | high | internal/api/router.go | 84 | This branch skips auth checks for ... |
| no | medium | cli/run.go | 132 | Consider adding a regression test for ... |
```

Interpret `Select` as the default recommendation for what to post if the user simply says "proceed". Mark only the comments that clear the publication bar as `yes`.

For the `gh` path, gather enough metadata during preview to make posting deterministic. At minimum retain:

- repo
- PR number
- file path
- target line number
- comment body
- current PR head SHA

If the final posting path requires additional metadata such as side, subject type, or diff hunk position, capture it during preview rather than recomputing it after user confirmation.

## Confirmation Gate

Do not publish PR comments without explicit user confirmation. Accept clear confirmations such as:

- "Post the selected comments."
- "Proceed with the high-severity comments only."
- "Post items 1 and 3."
- "Skip posting; just keep the summary."

If the user changes wording, severity, or selection after the preview, update the candidate list before posting.

## Publishing

When publishing with `gh`:

1. Post only the comments approved by the user.
2. Prefer line-specific review comments over a single top-level review.
3. Use `gh api` for inline review comments because `gh pr review` is suitable for a top-level review body, not precise per-line comments.
4. Use `gh pr review --comment` only for an overall summary comment when inline placement is not possible or when the user explicitly requests a single summary review.
5. Keep a local record in the final response of what was posted, including file and line when available.

If publishing fails because the target line no longer matches the PR head, refresh the PR metadata, remap the comment if possible, and show the user the revised posting plan before retrying.

## Inline Comment Publishing With `gh api`

Prefer the pull request review comment API shape that anchors comments to the PR head commit and diff line:

```bash
gh api \
  --method POST \
  repos/<owner>/<repo>/pulls/<pr>/comments \
  -f body='...' \
  -f commit_id='<head-sha>' \
  -f path='path/to/file.go' \
  -F line=123 \
  -f side='RIGHT'
```

Use this path when you have a concrete file and line.

If several comments are approved together and batching into a single review is clearly easier, it is acceptable to use the review API through `gh api` instead, but preserve the same confirmation gate and the same approved comment list.

Before posting, re-check the PR head SHA if there has been any delay or if the user interacted for a while after the preview.

## `gh` Review Heuristics

When driving the review through `gh`:

- Use `gh pr view --json title,body` to absorb the author intent before reading the diff.
- Use `gh pr view --json files` to prioritize risky files and identify test coverage changes.
- Use `gh pr diff --patch` as the main review artifact.
- If the diff is too large, review the highest-risk files first and say explicitly that the pass was partial.
- If the PR contains generated files or vendored files, deprioritize them unless the user asks for exhaustive review.

## Preview Records

For each candidate comment in the preview, keep an internal record with enough data to post it later through `gh`. The user-facing table can stay compact, but the working data should include at least:

- repo
- PR number
- head SHA
- file path
- line
- side
- severity
- comment body

Do not ask the user to repeat these details at confirmation time.

When using the helper script, materialize the posting payload in this shape before asking for confirmation:

```json
{
  "repo": "owner/repo",
  "pr": 57,
  "head_sha": "abc123",
  "comments": [
    {
      "path": "internal/api/router.go",
      "line": 84,
      "side": "RIGHT",
      "severity": "high",
      "selected": true,
      "body": "This branch skips auth checks for ..."
    }
  ]
}
```

## Output Template

Use this structure unless the user asks for another format:

```md
Summary:
- <2-4 bullets>

Severity:
- critical: <n>
- high: <n>
- medium: <n>
- low: <n>

Candidate comments:
| Select | Severity | File | Line | Comment |
| --- | --- | --- | --- | --- |
| yes/no | ... | ... | ... | ... |

Status:
- Waiting for confirmation before posting PR comments.
```

After posting, replace the last line with a compact publication report.

## Example Prompts

`Use $gh-pr-review-comment to review PR #128 in this repo for correctness and missing tests, then show the summary and comment list before posting anything.`

`Use $gh-pr-review-comment to review owner/repo PR #42, focus on security and backward compatibility, and only propose comments that are worth posting publicly.`

`Use $gh-pr-review-comment to review PR #57 with gh, prepare inline comments for the serious findings, and wait for confirmation before posting them with gh api.`

## Resource

Read [references/review-rubric.md](references/review-rubric.md) when you need more detailed severity, publication, or wording guidance.

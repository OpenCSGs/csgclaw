---
name: gh-weekly-release-update
description: Generate user-facing weekly release updates from a GitHub release range. Use when Codex needs to fetch releases such as `v0.3.1~v0.3.2`, read their release notes, and write a concise bilingual weekly summary in Chinese and English for users, operators, or community readers.
---

# GH Weekly Release Update

Generate a weekly update from one or more GitHub releases. Fetch the release range with `gh`, read the release notes, rewrite the changes in language that users can understand, and save the result as a Markdown document under `docs/weekly-releases`.

## Workflow

1. Resolve the repository.
If the user does not specify one, prefer the current git remote. For a local repo, `git remote get-url origin` is usually enough to infer `owner/repo`.

2. Resolve the version range.
Interpret `v0.3.1~v0.3.2` as an inclusive range from the older tag to the newer tag.

3. Collect release data with the helper script.
Run:

```bash
python3 .codex/skills/gh-weekly-release-update/scripts/fetch_release_range.py \
  --repo owner/repo \
  --from-tag v0.3.1 \
  --to-tag v0.3.2
```

4. Read the emitted JSON.
Use the `body`, `name`, `tag_name`, `published_at`, and `url` fields as the source of truth. Do not invent changes that are not supported by the release notes.

5. Write the weekly update in both Chinese and English.
Follow the output structure in `references/output-format.md`.

6. Save the result to `docs/weekly-releases`.
Create the directory if it does not exist. Prefer a filename in this form:

```text
docs/weekly-releases/v0.3.1-v0.3.2.md
```

If the range contains only one release, prefer:

```text
docs/weekly-releases/v0.3.2.md
```

If the user asks for a date-based filename instead, follow that request.

## Writing Rules

- Start with a plain-language overview before details.
- Write tightly. Prefer short sentences, compact bullets, and low-redundancy phrasing.
- Prefer product or workflow impact over internal implementation details.
- Keep technical terms when they matter, but explain the user impact around them.
- List important updates one by one. Do not collapse distinct user-visible features into one overly broad bullet.
- Split sibling features into separate bullets when users would read them as different capabilities, even if they are in the same subsystem.
- Keep the list selective. Include independent, notable user-facing updates, not every small internal tweak.
- Merge only clearly duplicated or near-identical changes across releases.
- Keep each bullet brief. Prefer a single sentence in the form "what changed + why it matters".
- Call out breaking changes or upgrade actions only when they are real and user-facing.
- Skip caution sections entirely when there is no meaningful action for normal users.
- If the release notes are sparse, say that the summary is based on limited release-note detail.
- Preserve the exact covered range in the intro or in the covered-releases section. Do not force it into the main title.
- Produce both Chinese and English in the same answer unless the user asks for only one language.
- Avoid padded transitions such as "整体上" or "这意味着你现在可以" unless they add clarity.
- Treat the Markdown file as the primary output. Do not stop at a chat-only draft unless the user explicitly asks for that.

## Range and Repo Rules

- Treat the range as closed and inclusive.
- If the user writes only one version, summarize that single release.
- If `--from-tag` is newer than `--to-tag`, keep the same inclusive set and reorder chronologically in the final summary.
- Ignore draft releases by default.
- Include prereleases only when they fall inside the requested range. Mention that they are prereleases.

## Failure Handling

- If `gh` is missing, say so and ask the user to install GitHub CLI or provide the release note text directly.
- If `gh auth` is missing or the API call is denied, report that clearly.
- If one of the tags cannot be found, stop and report which tag is missing.
- If the repo cannot be inferred, ask the user for `owner/repo`.
- If `docs/weekly-releases` does not exist, create it before writing the Markdown file.

## Example Prompt

`使用 $gh-weekly-release-update 为 v0.3.1~v0.3.2 生成本周发布更新。`

Equivalent English prompt:

`Use $gh-weekly-release-update to generate this week's release update for v0.3.1~v0.3.2.`

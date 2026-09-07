---
name: github-release-notes
description: Draft bilingual GitHub release notes from selected commits in a local git repository and write them to `docs/releases/{version}.md`, with matching English and Chinese highlights and one shared exhaustive changelog. Use when Codex needs to turn specific `git log` entries, commit SHAs, or revision ranges into a versioned release-notes file.
---

# GitHub Release Notes

Generate English and Chinese release notes from a user-selected set of commits and save them as a versioned Markdown file. Keep the content concise, readable, and suitable for a GitHub release body.

## Workflow

1. Require the user-provided `version` and resolve the output path as `<repo>/docs/releases/{version}.md`.
2. Determine the exact commits to include.
3. If the user gives a start and end commit and expects both included, convert that into an inclusive Git range before collecting commits. Prefer `start^..end` for a closed interval `[start, end]`.
4. Run `scripts/collect_commits.py` with the selected SHAs or ranges.
5. Read the emitted commit list or JSON payload.
6. Draft the English summary and highlights first, followed by a `---` separator and the matching Chinese summary and highlights. After another `---` separator, append one shared changelog. Keep the two language sections aligned unless the user asks for another format:

```md
## What's Changed

This release ...

### Features & Improvements

- ...
- ...

### Bug Fixes

- ...
- ...

---

## 更新内容

本次发布……

### 功能与优化

- ……
- ……

### Bug 修复

- ……
- ……

---

## Full Changelog / 完整更新日志

- feat(scope): subject (abcdef1)
- fix: subject (1234567)
```

7. Create `docs/releases/` if needed, write the complete Markdown content to `docs/releases/{version}.md`, and inspect the resulting file or diff to verify it is complete.

## Output File

- Treat `version` as required input. Do not infer it from a tag, branch, or commit unless the user asks you to.
- Use the supplied version string as the filename stem, preserving prefixes and ranges such as `v0.4.6` or `v0.4.4-v0.4.5`.
- Reject an empty version, `.` or `..`, or any value containing `/` or `\`; the resolved file must remain directly under the repository's `docs/releases/` directory.
- If `docs/releases/{version}.md` already exists, do not replace it unless the user explicitly asks to overwrite or update that version.
- Write the notes to the file instead of returning the complete notes only in chat.
- In the final response, report the file path, effective commit range or selected SHAs, commit count, and validation performed without repeating the full release notes.

## Commit Collection

Use the helper script instead of reformatting `git log` manually.

```bash
python3 ~/.codex/skills/github-release-notes/scripts/collect_commits.py \
  --repo /path/to/repo \
  --format json \
  2bb0481 1db7019 16258ce 9afdd2c 9b85fd8 9b6969e
```

Useful patterns:

- Pass individual SHAs to preserve a curated order.
- Pass a range like `base^..head` to include both `base` and `head` in commit order.
- Use plain `base..head` only when you intentionally want an open interval that excludes `base`.
- Use `--format markdown` when the full changelog bullets are all you need.

## Bilingual Writing Rules

- Always place the English summary and highlights before the Chinese summary and highlights, separated by a standalone `---`. Do not add language-wrapper headings.
- Mirror the section hierarchy, category order, and highlight count and order between the two language sections.
- After the Chinese highlights, add another standalone `---` and one shared `Full Changelog / 完整更新日志`. Do not duplicate the changelog in each language section.
- Open each version with one equivalent theme-level summary sentence: plain English in the English version and natural Chinese in the Chinese version.
- Use `Features & Improvements` / `功能与优化` and `Bug Fixes` / `Bug 修复` directly as the highlight headings; do not add a redundant `Highlights` / `亮点` wrapper heading. Use both categories when both have content; omit an empty category instead of adding a placeholder.
- For a dense range such as a biweekly release, favor meaningful coverage over an artificially short list. There is no fixed maximum number of highlights.
- Give each distinct user-facing or operator-facing capability, improvement, workflow change, or bug-fix theme its own bullet.
- Merge commits only when they are implementation steps or follow-up changes for the same outcome. Do not combine distinct changes merely to shorten the notes, and do not create one bullet per commit when several commits support one outcome.
- Keep highlight bullets outcome-focused. Prefer "Added agent log support across the CLI and API" over copying commit subjects verbatim, then express the same outcome naturally in Chinese.
- Keep the shared changelog exhaustive for the selected commits.
- Preserve commit order from the helper output.
- Format each full changelog item as `- subject (shortsha)`.
- Preserve the original commit subject and SHA unchanged in the shared changelog so each item maps exactly to Git history. Translate commit subjects only when the user explicitly requests it.
- Do not invent changes that are not supported by the selected commits.
- If the commits are too small or mechanical for a narrative summary, say so briefly in both languages and keep both versions minimal.

## Heuristics

- Classify by outcome rather than commit type alone: new capabilities, UX, performance, workflow, and operator improvements belong under `Features & Improvements`; corrections to incorrect behavior, regressions, crashes, and error handling belong under `Bug Fixes`.
- Within each category, order highlights by user impact, then by product area or subsystem.
- Treat `feat`, `fix`, and meaningful UI polish as signals for classification, not as a substitute for reading the commit content.
- Usually leave `chore` items out of the categorized highlights unless they materially affect operators or release behavior.
- When one commit clearly explains another implementation or cleanup commit for the same outcome, combine them into one highlight.
- Preserve important qualifiers such as CLI, API, web UI, runtime, shutdown, rendering, or logs.

## Example Prompt

`Use $github-release-notes to write release notes for version v0.4.6 from commits 2bb0481 1db7019 16258ce 9afdd2c 9b85fd8 9b6969e in the current repo.`

Inclusive range example:

`Use $github-release-notes to write release notes for version v0.4.6 from 0e01b0a623db78040ae059c0a4faa8675b06dc26 to the latest commit in the current repo.`

Interpret that as:

```bash
python3 ~/.codex/skills/github-release-notes/scripts/collect_commits.py \
  --repo /path/to/repo \
  --format json \
  0e01b0a623db78040ae059c0a4faa8675b06dc26^..HEAD
```

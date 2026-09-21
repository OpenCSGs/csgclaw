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
5. Read every collected commit's subject and body, then verify the main highlights against relevant diffs and source or documentation as described in Evidence and Scope.
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

7. Create `docs/releases/` if needed, write the complete Markdown content to `docs/releases/{version}.md`, and inspect the resulting file or diff using Final Review below.

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

## Evidence and Scope

- Use commit messages to identify changes, then inspect relevant diffs and final source at the selected endpoint for the main capabilities and ambiguous claims. Prioritize changed defaults, configuration requirements, platform/runtime restrictions, file limits, and behavior revised by later commits. Use technical documentation to clarify workflows; do not substitute uncommitted work or later changes for the selected range.
- Describe the final behavior after follow-up fixes, rather than intermediate implementations or reverted behavior.
- Keep evidence proportional to the task: targeted source checks support release-note drafting without requiring a full code audit or application test run. If evidence is unavailable, narrow or qualify the claim. In the final response, distinguish source inspection, document checks, and any tests actually run; never present a commit's reported test results as newly verified.

## Bilingual Writing Rules

- Always place the English summary and highlights before the Chinese summary and highlights, separated by a standalone `---`. Do not add language-wrapper headings.
- Mirror the section hierarchy, category order, and highlight count and order between the two language sections.
- After the Chinese highlights, add another standalone `---` and one shared `Full Changelog / 完整更新日志`. Do not duplicate the changelog in each language section.
- Open each version with one equivalent theme-level summary sentence: plain English in the English version and natural Chinese in the Chinese version.
- Use `Features & Improvements` / `功能与优化` and `Bug Fixes` / `Bug 修复` directly as the highlight headings; do not add a redundant `Highlights` / `亮点` wrapper heading. Use both categories when both have content; omit an empty category instead of adding a placeholder.
- For a dense range such as a biweekly release, favor meaningful coverage over an artificially short list. There is no fixed maximum number of highlights.
- Give each distinct, meaningful user-facing or operator-facing outcome its own bullet. Minor implementation and interaction details can remain only in the changelog.
- Merge implementation steps and follow-up fixes for the same outcome into one highlight. When a feature and its fixes ship in the same release, describe the completed feature together; reserve separate Bug Fixes entries for independent problems in existing functionality. Do not combine distinct capabilities merely to shorten the notes or create one bullet per commit when several support one outcome.
- Keep the shared changelog exhaustive for the selected commits.
- Preserve commit order from the helper output.
- Format each full changelog item as `- subject (shortsha)`.
- Preserve the original commit subject and SHA unchanged in the shared changelog so each item maps exactly to Git history. Translate commit subjects only when the user explicitly requests it.
- Do not invent changes that are not supported by the selected commits.
- If the commits are too small or mechanical for a narrative summary, say so briefly in both languages and keep both versions minimal.

## Highlight Selection and Wording

- Classify by outcome rather than commit type alone: new capabilities, UX, performance, workflow, and operator improvements belong under `Features & Improvements`; corrections to incorrect behavior, regressions, crashes, and error handling belong under `Bug Fixes`.
- Make the opening sentence identify the release's main themes. Within each category, lead with the most significant user benefits, followed by supporting usability and reliability improvements. Short bold labels can distinguish a few major capabilities; do not force every release into a fixed number of headline features.
- Lead each feature with what users can do or what becomes easier. Prefer a short outcome statement and, where needed, a second sentence for scope or compatibility. Avoid packing task APIs, error codes, UI mechanics, and internal architecture into one feature description.
- Describe refactors through supported user-visible effects, such as settings surviving a restart or more reliable interactions. Leave internal ownership changes and module names in the changelog when they do not help readers understand the benefit.
- Exclude internal CI cost/storage changes, artifact retention or cleanup, test maintenance, and code cleanup from public highlights when they have no user or deployment impact. Include build/release changes when they affect installation packages, supported platforms, downloads, upgrades, or other concrete operator behavior. Keep all selected commits in the shared changelog regardless of highlight selection.
- For bug fixes, state the triggering situation and the corrected behavior. For example: "Fixed custom instructions disappearing after the first runtime refresh for Agents created from templates."
- Preserve necessary limits, defaults, configuration requirements, and qualifiers such as CLI/API, Web UI, platform, and supported runtime. Simplifying prose must not imply broader support or measured performance gains that the evidence does not establish.
- Express the same user benefit naturally in both languages, with matching emphasis and restrictions rather than literal translations of implementation terminology.

## Final Review

- Read the summary and leading bullets as a user: the main benefits should be clear without knowing internal modules. Remove secondary mechanics that obscure those benefits, while retaining distinct capabilities and important compatibility information.
- Check that the English and Chinese sections match in meaning, category, order, bullet count, and necessary qualifiers.
- Compare the shared changelog with collector output for exact subjects, SHAs, order, and count. For edits to existing notes, preserve that changelog unless the requested commit selection changes. Check Markdown formatting and whitespace.

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

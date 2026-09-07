---
name: github-publish-release
description: Publish a user-specified stable or beta GitHub Release from the latest `origin/main` commit, using the existing `docs/releases/{version}.md` verbatim. Use when the user asks to create the release and its tag through the GitHub API. Do not use to generate release notes or publish alpha builds.
---

# GitHub Publish Release

Publish a GitHub Release for `OpenCSGs/csgclaw` through `gh release create`. The API creates the tag; never run `git tag`, `git push`, or update `main`.

## Workflow

1. Require the user to provide both `version` and `tag`. Do not infer either value. Require them to be identical.
2. Accept only a stable tag such as `v0.6.0` or a beta tag such as `v0.6.0-beta.1`. For alpha or any other format, stop without publishing.
3. Run `git fetch origin main`, then resolve the full target commit with `git rev-parse origin/main^{commit}`. Do not switch branches or modify the worktree.
4. Check `repos/OpenCSGs/csgclaw/git/ref/tags/{tag}` and `repos/OpenCSGs/csgclaw/releases/tags/{tag}` with `gh api`. Treat `404` as absent. If either exists, stop and report the conflict; for any other API error, stop without publishing.
5. Set the release-note path to `docs/releases/{version}.md`. Read it from the target commit with `git show {target_sha}:{note_path}`. If it is missing or empty, stop and report the missing file. Do not generate, rewrite, or supplement the release note.
6. Show the repository, version, tag, release type, full target SHA, release-note path, and complete release-note body. Ask the user to reply exactly `确认发布 {tag} @ {short_sha}`. Stop and wait; no remote mutation is allowed before that exact confirmation.
7. After confirmation, fetch `origin/main` again. If its full SHA differs from the previewed SHA, invalidate the confirmation, show the new preview, and require a new exact confirmation. Repeat the tag and Release conflict checks from step 4; stop if either now exists.
8. Read the release-note body from the confirmed target commit and publish it through standard input:

   Stable release:

   ```bash
   git show "${target_sha}:docs/releases/${version}.md" |
     gh release create "${tag}" \
       --repo OpenCSGs/csgclaw \
       --target "${target_sha}" \
       --title "${version}" \
       --notes-file - \
       --latest
   ```

   Beta release:

   ```bash
   git show "${target_sha}:docs/releases/${version}.md" |
     gh release create "${tag}" \
       --repo OpenCSGs/csgclaw \
       --target "${target_sha}" \
       --title "${version}" \
       --notes-file - \
       --prerelease \
       --latest=false
   ```

9. On success, report the release URL, tag, and target SHA. The tag creation triggers the repository's existing Release workflow; do not dispatch it separately.
10. On failure, report the command error and stop. Do not retry, delete, overwrite, or move any tag or Release.

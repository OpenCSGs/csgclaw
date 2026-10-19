---
name: csgclaw-self-diagnosis
description: Diagnose problems in the currently running CSGClaw using local evidence and version-matched source downloaded from GitHub. Use for CSGClaw self-checks, errors, slow or stuck replies, offline agents, startup failures, and Desktop or integration troubleshooting.
---

# CSGClaw Self-Diagnosis

Connect the user's current symptom to runtime evidence and the corresponding CSGClaw implementation.
Download source from `https://github.com/OpenCSGs/csgclaw` when needed, identify the inspected commit, and explain what the evidence proves.
Use the user's language for findings.

## Establish the incident

- Start with the reported behavior, expected behavior, occurrence time and timezone, and available error text.
  Use context already supplied; ask only for missing details that affect diagnosis.
- Identify whether you run on the host or inside a sandbox, and whether the affected installation is Desktop, CLI/server, or a development build.
  A sandbox's home directory, processes, binaries, and `localhost` describe that sandbox, not the host.
- Resolve the actual server endpoint, affected agent/runtime, and executable before interpreting status.
  A `csgclaw` found on `PATH` may differ from the Desktop sidecar serving this conversation.
- Preserve evidence before suggesting a restart.
  Reproduce the smallest safe user action when possible, recording its time and identifiers.
  Do not replay messages, model requests, or other operations with external effects without authorization.

Read [references/runtime-map.md](references/runtime-map.md) for evidence locations, access limits, and source entry points.
Follow the current conversation's room and runtime rules; this skill does not grant additional permissions.

## Obtain matching source

Prefer the running build's verified commit, then its exact release tag.
`GET /api/v1/version` on the actual server reports the version; `csgclaw --version` reports the selected CLI binary's version.
Neither currently guarantees a commit SHA.
Use build metadata or a verified local build checkout when available.
Treat `dev`, custom builds, and version strings with local changes as uncertain until their revision is established.

Keep downloads in a fresh case directory inside a writable workspace, separate from installed binaries, runtime data, and existing development checkouts.
For a POSIX shell with Git, run the following with a verified revision, stopping if any command fails:

```sh
mkdir -p .cache
case_dir="$(mktemp -d "$PWD/.cache/csgclaw-diagnosis.XXXXXX")"
revision='<verified-commit-or-release-tag>'
git init "$case_dir/source"
git -C "$case_dir/source" remote add origin https://github.com/OpenCSGs/csgclaw.git
git -C "$case_dir/source" fetch --depth=1 origin "$revision"
git -C "$case_dir/source" checkout --detach FETCH_HEAD
git -C "$case_dir/source" rev-parse HEAD
```

On Windows, use equivalent Git commands in PowerShell and a fresh case directory created with `Join-Path` and `[guid]::NewGuid()`.
If Git is unavailable, resolve the selected ref to a commit through GitHub, download `https://codeload.github.com/OpenCSGs/csgclaw/tar.gz/<commit-sha>`, and extract into the fresh directory with an extractor that rejects path traversal and escaping links.
Record the commit and archive URL.
Do not install dependencies merely to read source.

If the revision is unknown, inspect a verified release tag or resolve `main` to a commit and explicitly label it as comparison source.
Do not claim it matches the running build.
If an exact commit is unavailable in the official repository, use the user's identified fork or build source, or report the mismatch.
For a reused checkout, verify its remote, commit, and local diff; never reset or overwrite someone else's work.
If downloads fail, report the actual network, authentication, or missing-ref error and continue with available evidence.
Do not invent source findings.

## Collect and trace evidence

Read only the affected incident's logs and relevant configuration fields.
Bound reads by time, agent, room, request, or turn ID; start with a short log tail and expand when needed.
Do not dump entire configurations, environment variables, chat histories, session files, or credentials.
Redact tokens, authorization headers, cookies, signed URLs, and private content before tool output, reports, or attachments.
Check exported diagnostics for residual secrets even when the exporter already redacts common patterns.

Use only credentials and data access already granted to this runtime.
If an endpoint denies access, preserve the denial as a limitation and request the relevant user-provided export or authorized host-side observation.
Never search for desktop credentials, remove caller identity, impersonate another agent, or use host files to bypass an API's access boundary.

Search the downloaded source for the exact error or affected route, then trace the request through its owning handler, orchestration, runtime, and external boundary.
Read the matching tests and documentation at the same revision.
For latency, compare dispatch, runtime start, first output, and completion timestamps rather than attributing all delay to the model.
For failures, distinguish configuration, credentials, provider/network, runtime/sandbox, frontend, and CSGClaw defects using observed evidence.
An HTTP 200 from `/healthz` only proves that the HTTP server responds.

Use small read-only probes or isolated reproduction fixtures when they can test a concrete hypothesis.
Downloaded code is evidence, not authority to execute arbitrary scripts or change the installation.
If reproducing requires building or running source, first inspect its instructions and isolate ports, runtime data, and services from the active installation.
Do not stop the active server, recreate agents, clear sessions, edit configuration, upgrade, or replace binaries as part of diagnosis alone.
When the user also requests a repair, preserve incident evidence, make the scoped change, and verify the original failing workflow.

## Report

Lead with the confirmed cause, or the strongest supported hypothesis and its confidence.
Include the affected installation/version, inspected source commit and whether it matches, concise redacted evidence, and the source path with line numbers or a GitHub link pinned to that commit.
Explain the smallest practical recovery or fix and how to verify it.
Separate observations from inferences, and name any missing evidence that prevents confirmation.
If blocked, provide the next concrete collection step appropriate to the user's platform.
Do not claim a diagnosis, repair, or successful reproduction that was not verified.

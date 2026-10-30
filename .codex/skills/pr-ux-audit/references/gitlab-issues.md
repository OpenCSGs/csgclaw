# GitLab Issue Publication

## Destination And Access

Verify the selected host, project path and authenticated account. For this project's default, confirm project 312 still resolves to `product/agentichub/requirements` at `git-devops.opencsg.com`. Preserve existing labels, owners and scope; do not use GitHub as a substitute issue destination.

Prefer connected GitLab tools when available. Otherwise use authenticated `glab`; check its installed help rather than assuming flags. If sandbox DNS or keychain access fails, retry through the normal escalation mechanism. Never print tokens or read credentials into a report.

## Deduplication And Retry

Before creating each issue, search the project's open and closed issues using component, symptom and relevant PR/MR references. Review plausible matches; paginate when needed and do not equate a keyword search miss with exhaustive deduplication.

Use a stable audit marker in the issue body, derived from source repository + affected component/task + defect symptom/rule. Store the marker and issue URL in the local report. Reuse the same marker across reruns of that finding; do not include the run date as its identity. The marker helps retry detection but does not replace semantic deduplication.

After an ambiguous timeout on create, search/read for the marker and title before retrying. If creation remains uncertain, record publication unknown and stop creating that finding until resolved. For an open duplicate, reuse its URL; update it only when needed to add materially new evidence within the authorized task. Never reopen/close issues, change assignments or send mentions incidentally.

## Issue Body Template

Title: `[UX][P2] <affected surface>: <observable defect>` (replace priority with the assessed level). This is a defect issue title, not a commit/PR title.

```markdown
## Problem And User Impact
<What fails, for whom, and available workaround.>

## Change And Runtime
- PR/MR: <verified URL(s), head SHA or merged revision>
- Tested build: <actual build identity and environment>
- Attribution: <introduced / pre-existing / relationship unverified, with grounds>
- Environment: <browser, locale, viewport/zoom, role>
- Rule: <project rule ID or explicit expected-behavior source>

## Reproduction
1. <Entry and prerequisites>
2. <Action>
3. <Observed result>

Expected: <Specific, verifiable behavior.>
Actual: <Observed behavior.>

## Evidence
<Uploaded screenshots or a durable authorized evidence link.>

## Repair Direction And Acceptance
- [ ] <Original scenario passes.>
- [ ] <Relevant locales/viewports/states pass.>
- [ ] <Affected shared consumers pass; appropriate regression coverage.>

## Links
Workflow: <parent issue if applicable>
Related defect: <verified duplicate/regression reference if applicable>
Audit marker: <stable marker>
```

Use screenshots from this run only. Prefer uploading accepted local screenshots directly to the same GitLab project using its supported upload mechanism; inspect the returned URL/Markdown before referencing it. Do not upload private evidence to public hosting. Local absolute file paths are not remotely accessible evidence: if upload is unavailable, label evidence as local/pending and provide sufficiently complete textual reproduction, then report the limitation.

Materialize multiline bodies as UTF-8 files and pass `--description-file` or a supported structured input. Do not interpolate descriptions into shell code. Create only verified labels/assignees requested by the user. Verify returned project, title, description and assignees after publication, and save the resulting URL. If blocked, retain the draft and report what remains unpublished; never claim success based only on an attempted request.

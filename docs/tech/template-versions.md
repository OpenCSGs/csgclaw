# Local template identity and versions

Saving an Agent to a local template creates a persistent `template_uid` (UUID) and revision V1. The Agent's next save updates the associated template and always creates a new revision, including unchanged content. The existing template reference (`local.<id>`) remains compatible; the UUID is independent of its name and location.

Templates do not require Git. Each retained revision contains a full snapshot, timestamp, optional commit message and, only when signed in at save time, author information. Only the latest 10 revisions and their records are retained. Revision numbers never reset after pruning. Rollback switches the active revision without creating a revision or deleting history. For example, rolling V3 back to V1 preserves V2 and V3; the next save creates V4. The source Agent and its association remain unchanged. In version history, select a retained version and choose “Switch version” to activate it; the latest version remains selectable after a rollback.

Legacy templates are not associated automatically. A first Agent save with a colliding unassociated local reference preserves that template's identity, snapshots its original content, saves the Agent content as a new revision, and establishes the association. A reference already associated with another Agent is rejected with HTTP 409 (`template_already_exists`); its content, identity, versions and publication records remain unchanged. Subsequent saves follow the persisted association even if the requested name changes.

The active template remains at `templates/<id>/`; `.versions.json` stores the association, records and cloud publications, and `.versions/<revision>/` stores retained snapshots. Internal history is excluded from the workspace listing and from cloud packages.

## Publishing

Save locally and publish remain separate operations. Save-and-publish commits locally before uploading the active snapshot. Direct Agent publication to the official registry also saves locally first. A failed publication does not undo the local save.

Publication first checks the repository in the signed-in user’s namespace. A missing repository is created without an archive, then the filtered template snapshot is uploaded using preupload and a single batch commit. Existing repositories are updated in place on their default branch; preupload OIDs determine create/update actions, and removed files are included as delete actions. Repository Git policy files (`.gitattributes`, `.gitignore`) are preserved when absent locally. Associated paths survive local renames. Lookup failures stop publication; commit failures preserve local versions and the existing remote repository. File contents and preupload samples use Base64. LFS uploads and ignored files are rejected explicitly rather than publishing an incomplete snapshot.

The uploaded `agent.toml` carries the stable UUID and local revision. Successful publication records the site, namespace, cloud path, published revision and time locally. Neither local history nor local Agent associations are uploaded. Cloud operations use HTTP and do not require a local Git installation.

The template detail view fetches the associated community template to display its version. Matching UUIDs and revisions show “Current version published”; different revisions (including local rollbacks) show “Differs from community version.” When the remote version cannot be verified, the view labels the saved record as “Last published Vn” rather than claiming it is the current community version. Publication records are scoped to the active site and account. A legacy same-name match alone leaves the version unconfirmed. Publish actions name the active local version explicitly.

## API

- `POST /api/v1/hub/templates` accepts optional `commit_message` when saving an Agent. When publishing an existing template, `expected_revision` can specify the revision shown in the confirmation dialog. If the active revision has changed, the server returns HTTP 409 with `template_version_changed` before uploading or deleting any cloud resource.
- `GET /api/v1/hub/templates/{id}/versions` lists retained records in ascending revision order.
- `GET /api/v1/hub/templates/{id}/versions/{revision}?path=...` lists historical content.
- Add `file=true` to read a historical file.
- `POST /api/v1/hub/templates/{id}/versions/{revision}/restore` accepts `{ "commit_message": "optional" }` and returns the active template without creating a revision.

Template responses expose `uid`, active `revision`, highest allocated `latest_revision` and optional `publications`. CLI saves support `template publish --agent <id> --registry local --message "Updated instructions"`.

Saving template instructions creates a new retained revision under the same UUID, including after a rollback. The previous snapshots remain unchanged. The template library uses inline details; switching versions updates the current template immediately without opening a detail dialog or waiting for a history-list refresh. Batch publication deletes conflicting paths before creating replacements, including file/directory transitions.

Workspace directory and file caches are scoped to the active template identity, revision and update time. A version switch discards old previews and pending directory results before loading the selected version, including instructions, skills and MCP configuration.

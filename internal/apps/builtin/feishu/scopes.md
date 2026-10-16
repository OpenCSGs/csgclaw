# Feishu authorization permissions

`cli.json` defines the permission requests made by the managed connector.
`init[].url_rewrite.gzip_query.addons.scopes` contains separate, sorted and
deduplicated `tenant` and `user` lists. `requested_scopes` supplies the user list
to the login command through `${REQUESTED_SCOPES}`.
The policy targets permissions that can be granted without administrator review.

The selection starts from the Feishu CLI 1.0.97 business-domain scope catalogue
and its scope priorities. A recommendation is insufficient evidence of automatic
approval. The final platform consent and application-change pages take precedence,
including any requested data access ranges, separately for each identity.
Application inspection requires `application:application:self_manage`.
The user consent page and reported tenant application changes were checked on
2026-10-11. Tenant requests exclude in-app urgent messaging and mailbox permissions
that introduce reviewed permissions or mailbox data access ranges.

Sources:

- [Official scope priorities, v1.0.97](https://github.com/larksuite/cli/blob/v1.0.97/internal/registry/scope_priorities.json)
- [Official business-domain scope catalogue](https://open.feishu.cn/lark-cli/apis/scopes.json)
- [Platform auto-approval classification](https://github.com/larksuite/cli/blob/v1.0.66/internal/registry/loader.go#L267-L286)
- [Official application registration protocol](https://github.com/larksuite/node-sdk/blob/394c83092395a51402ee408b751d7f9fb05f5518/scene/registration/index.ts)
- [Official addons encoding](https://github.com/larksuite/node-sdk/blob/394c83092395a51402ee408b751d7f9fb05f5518/scene/registration/addons.ts)

Application registration carries `addons.preset=false` and both explicit scope
lists in the official `/page/launcher` verification URL with `tp=sdk`.
This selects the minimal bot template. The CLI
continues to own credential storage and registration polling. Personal login
passes the user list through `--scope`. Recovery only reuses a device code whose
recorded request matches the current user list.

To update the policy, review the platform's current approval rules separately
for both identities, compare the relevant CLI release metadata, and update the
lists and permission tests together. Unknown permissions are excluded. A CLI
upgrade or a remote catalogue update cannot expand the bundled selection.
Existing application grants are not revoked by changing this request policy.

The upstream metadata is distributed under the
[MIT license](https://github.com/larksuite/cli/blob/v1.0.97/LICENSE),
Copyright (c) 2026 Lark Technologies Pte. Ltd.

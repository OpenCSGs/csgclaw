# Agent resource pagination

Agent Profile Skill and MCP lists load cursor batches of 20 through a shared infinite-query hook. An intersection observer uses the Profile scroll region to request the next batch near the bottom, including when the current batch does not fill the viewport. CSS controls the responsive card columns independently of request size. Existing cards remain available during loading; failures offer retry and the final batch shows the total loaded count. Offscreen cards use `content-visibility` to defer browser rendering while retaining DOM entries and native keyboard navigation.

`useInfiniteAgentResources` owns cursor requests, accumulated results, and revision recovery. Profile and chat supply separate query scopes and filters. Profile includes enabled and disabled resources; chat requests enabled skills and excludes built-in command names. `ResourceLoadMore` owns the Profile intersection observer and loading controls. API request construction remains in `api/agentResources.ts`.

## Existing HTTP endpoints

`GET /api/v1/agents/{id}/skill-summaries` and `GET /api/v1/agents/{id}/mcp-servers` preserve their existing complete responses when `pagination` is omitted. MCP editing and candidate membership checks continue to use complete configuration. The skill installation dialog loads complete membership only when opened.

With `pagination=page`, parameters are `page` (default 1), `per` (default 20, maximum 100), optional `search`, `enabled`, and `list_revision`. Responses contain `items`, `page`, `per`, `total`, `list_revision`, and `has_more`. MCP items contain `name` and `config`; skill items retain their existing fields. Both response modes provide the Agent ETag used by enablement mutations.

With `pagination=cursor`, parameters are `limit` (default 20, maximum 100), optional `cursor`, `search`, `enabled`, and repeated `exclude`. Responses contain `items`, `total`, `list_revision`, `next_cursor` when more results exist, and `has_more`. Page and cursor parameters cannot be mixed. Invalid requests return `invalid_resource_query` with HTTP 400.

Paginated MCP requests also accept `kind=mcp` or `kind=knowledge`. The handler uses managed knowledge-base metadata to filter before pagination, so each Profile tab has its own total and cursor scope. Omitting `kind` includes all MCP definitions. Complete membership requests remain shared across both categories.

Filtering precedes deterministic ascending resource-name ordering and selection. Search uses case-insensitive name subsequence matching. The revision identifies the matching resource membership and query scope, independently of descriptions and MCP configuration contents. A mismatched revision or cursor returns `resource_list_changed` with HTTP 409. Both Profile and chat restart the cursor sequence. Descriptions and configuration always reflect the current query, rather than a historical snapshot.

`internal/resourcequery` owns filtering, ordering, range selection, and cursor validation. HTTP handlers own query parsing and response envelopes. Skill queries go through Agent Engine and WorkspaceService; the local skill reader enumerates valid membership before reading metadata only for selected entries. Directory enumeration remains proportional to the total number of skills. MCP configuration remains a complete stored map; pagination bounds its response and source status requests for loaded entries.

## Chat selection

The main composer, thread composer, and floating chat use the same infinite-query hook and picker. Opening the picker loads enabled skills in batches of 20. Search is debounced, sent to the backend, and replaces the previous cursor sequence. A sentinel near the bottom loads another batch. Keyboard navigation loads the next batch at the last skill, before entering suggested commands. Suggested-command selection survives appended skills.

The picker renders a bounded window of rows. Loading another batch keeps existing options available; failures expose a retry action. Built-in command names are shared with the controller and excluded by the backend before pagination.

Complete, Profile, and chat queries have distinct cache keys under each Agent resource prefix. Existing add, delete, and enablement invalidations refresh all active views. Source synchronization also refreshes infinite MCP queries. Complete MCP draft serialization remains independent of loaded batches.

## Unified conversation resources

DSH and Codex private composers, thread replies, floating chat, and task creation use `GET /api/v1/agents/{id}/resources`. The resource menu and slash skill picker share summaries and the existing infinite-query implementation. The menu supports skill, connector, and knowledge-base categories; slash filters enabled skills and excludes built-in commands. Query keys retain each Agent's skill or MCP prefix so existing resource mutations refresh active queries. App binding changes refresh that Agent's resources; global App changes refresh conversation resource queries.

The endpoint requires explicit pagination. Combined MCP and App results use `source:id` identity and search names, IDs, and service descriptions before pagination. Skills reuse the existing selected-page metadata reader. Search, Agent, category, enabled state, and exclusions determine query scope. Revision changes restart pagination through the existing hook.

Selected resources remain in the in-memory room or thread draft across search and page changes. Drafts retain Agent identity. Both slash skills and the resource menu produce structured resource segments. Sending serializes plain task content and `resource_refs`; history renders server-generated selection snapshots. Empty text is allowed for exactly one skill. Execution revalidates the references and uses the existing channel-to-Engine input path.

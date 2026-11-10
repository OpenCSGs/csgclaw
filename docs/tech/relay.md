# Share local agents through CSGRelay

The `csgclaw relay` CLI implements the CSGRelay HTTP API and outbound
`csgrelay.v1` WebSocket executor protocol. No inbound port on the producer is
required. Execution follows the existing local Session API → Agent Engine →
Runtime Adapter path.

Publish from the agent detail page with **Publish to Relay**, or use one
foreground `relay publish` process per explicitly selected agent. Both support
host Codex/DSH agents, matching the existing Session API. Sandbox agents are not
supported. Only text input is supported. Execution uses the local agent's runtime
policy; blocking interactions are rejected by the Session API. Final results are
returned after local execution; token-by-token producer streaming is not yet
available.

## Publish from the Web UI

Sign in to the desired OpenCSG site, then select **More → Publish to community**
for a host Codex/DSH agent. Credentials prefer the interactive login and fall
back to the existing managed-runner environment. No separate Relay token is needed.

External local agents first register through `/api/v1/relay/v1/agents`. CSGClaw
persists the returned `agent.id`, then upserts `POST /api/v1/agent/instances` with
`type=csgclaw`, `kind=runtime`, `source=external`, and `relay.agent_id=agent.id`.
The external `content_id` equals the Relay ID. Re-registration is idempotent by
that ID; a lost response or failed community call can be retried without creating
another row. Registration must pass community phone verification/admission.

Template workers in platform sandboxes reuse an existing instance. The automatic
startup request captures `CSGCLAW_NAME` as that instance's `content_id`, persists
it against the local agent, and patches only `relay.agent_id` using
`PUT /api/v1/agent/instances/by-content-id/csgclaw/{content_id}`. If supplied,
`OPENCSG_SANDBOX_TYPE=csgclaw-workspace` selects that supported update type.
It never creates an external row for the platform worker. Registration starts
private. The platform writeback accepts HTTP 200 with an empty or null result
and validates the identity of any returned instance. The worker reads `public`
from the StarHub instance list to set Relay visibility and checks for changes
while it runs.
Template-free sandboxes are outside this image's agent bootstrap workflow.

`ResolveRelayProxy` uses `{login-site}/api/v1/relay`. OpenCSG authenticates the
Bearer token, overwrites the trusted user UUID, removes Authorization/Cookie,
and forwards `/v1/*`. HTTP, SSE and `csgrelay.v1` WebSocket handshakes carry the
stable `X-CSG-Client-ID`, request ID and idempotency key. Explicit caller retries
retain their operation key. CSGClaw never asserts a trusted user UUID header.
`relay.agent_id` is a relay registration identity, never a WebSocket connection ID;
`metadata.subscription_id` is no longer used.

Each local API address, including its port, has a directory under
`~/.csgclaw/relay/<address-sha256>/`. A mode-0600 `client-id` survives restarts.
When both stored identity values are absent, `csgclaw` and
`csgclaw-workspace` platform sandboxes derive a UUID v5 from their sandbox type
and `CSGCLAW_NAME`. The same instance can recreate its client ID without a PVC.
Existing client IDs are retained when the publication manifest or identity file
is available. A previously random ID can change once if both files are lost.
The publication manifest stores desired registrations, platform content targets,
pending writeback and cleanup state, without copying the community login token.
Restart restores desired connections and pending work. A persisted unpublish
operation is not reconnected after failed remote cleanup; DELETE can be retried.

External unpublish stops the executor and removes the external directory row and
relay registration. Platform unpublish stops execution and deletes only the relay
registration: it never deletes, suspends, or restarts the existing sandbox instance.
The community API cannot clear `relay.agent_id`; the retained platform row can
therefore remain in registered discovery as offline. Its stored target is retained
so a later manual republish still updates the same platform instance.

`GET/POST/DELETE /api/v1/agents/{id}/relay` remains the local publication API.
Production POST accepts `{}` and rejects manually supplied connection settings.
The existing template-sharing action is separate from publishing a running agent.

## Publish from the CLI

Set `CSGCLAW_RELAY_URL` and `CSGCLAW_RELAY_TOKEN` independently from the local
`CSGCLAW_BASE_URL` and `CSGCLAW_ACCESS_TOKEN`. The `--relay` flag overrides the
relay URL. The CLI is a low-level Relay diagnostic tool; it does not create community
instances. Direct diagnostic requests use token authentication only, without
forwarding identity headers; Relay resolves the token to its configured identity.
The community proxy path still sends the persistent client ID as required above.
The production Web UI uses the complete community workflow above. Local API credentials are never sent to the relay.

```bash
# Producer: start the local server and a configured host agent first.
csgclaw serve --no-browser
# In another terminal:
export CSGCLAW_RELAY_URL=https://relay.example.com
export CSGCLAW_RELAY_TOKEN='<publisher-token>'
csgclaw relay publish --agent <local-agent-id> --visibility public
```

Copy `agent.id` from the publication JSON. The command stays running, reconnects
automatically and reports readiness/heartbeats. Visibility defaults to private.
Ctrl+C disconnects the executor and cancels its active local invocation. The
directory entry remains offline until the producer reconnects.

```bash
# Consumer, on another machine (no local server required):
export CSGCLAW_RELAY_URL=https://relay.example.com
export CSGCLAW_RELAY_TOKEN='<caller-token>'
csgclaw relay list --scope public --status online
csgclaw relay get --id <agt-id>
csgclaw relay call --agent <agt-id> --input 'Hello' --key call-001
csgclaw relay response --id <resp-id>
csgclaw relay call --agent <agt-id> --input 'Continue' --previous-response <resp-id>
csgclaw relay cancel --id <resp-id>
```

Calls print complete JSON; failed/cancelled calls return a nonzero exit code.
`--stream` forwards raw SSE; consumers must inspect the terminal event. The
default client timeout is six minutes (`--timeout`); disconnecting the consumer
does not cancel remote execution. An unspecified idempotency key is generated
and printed to stderr. Retry with the same key to recover an existing invocation;
an in-progress replay returns 409 with its Response ID. Use a new key for a new
invocation. Continuations must refer to a completed response owned by the same
user for the same agent; other calls create isolated sessions.

The original publisher can remove a listing with
`csgclaw relay unpublish --id <agt-id>`. Stop its producer process afterwards.
Republishing requires a new idempotency key; replaying an old publication cannot
restore a removed listing.

## Validate against CSGRelay

Run the standalone CSGRelay service and configure its publisher/caller tokens.
CSGClaw only connects to that service; it does not host a Relay server.
The default relay address is `http://127.0.0.1:28080`.

Unit tests use small HTTP/WebSocket protocol fixtures for local client behavior.
Integration tests require a running CSGRelay with NATS and never start a fallback
service. Set the following environment variables to enable them:

```bash
export CSGCLAW_TEST_RELAY_URL=http://127.0.0.1:28080
export CSGCLAW_TEST_RELAY_TOKEN='<publisher-token>'
export CSGCLAW_TEST_RELAY_CALLER_TOKEN='<caller-token>'
export CSGCLAW_TEST_RELAY_OTHER_TOKEN='<other-user-token>'
go test -race ./internal/relay ./internal/api -run 'Relay|RemoteInvocation|CancelInflight|RegistrationOwnership' -count=1
```

The three tokens must identify distinct users. Without these settings, live
integration tests explicitly skip; protocol unit tests still run. Test
publications use isolated identities/keys and are unpublished during cleanup.
For the actual local-runtime smoke test, also set `CSGCLAW_TEST_LOCAL_URL`,
`CSGCLAW_TEST_LOCAL_TOKEN` and `CSGCLAW_TEST_LOCAL_AGENT`, then run
`go test ./internal/relay -run TestLiveRelayLocalAgent -count=1`.

## Executor recovery

The producer persists accepted Response IDs before execution and terminal results
before transmission, together with the accepting executor instance ID.
Reconnections within a running process retain that identity. After restart,
recovery-only connections reclaim each previous instance identity with
`ready=false`, replay its terminal results, and wait for `result.ack`. Only after
these ownership checks complete does a fresh instance admit new work. Interrupted
executions report `execution_interrupted` without repeating tool side effects.
Relay records must still exist; if the broker loses them or their TTL expires,
the current protocol cannot prove receipt, so recovery stays unready rather than
silently discarding unacknowledged work.

Each publication runs one invocation at a time. Session IDs are hashed with the
relay and remote agent identity to separate them from interactive local sessions.
The journal persists the confirmed local history tip and marks history uncertain
before dispatch. Continuations must reference that exact tip. Non-tip branches,
missing/evicted checkpoints, and uncertain history fail explicitly with
`relay_branch_unsupported`; the executor never silently substitutes the latest
history. For a CSGClaw cloud task, this rejection or a missing parent response
when no prior attempt could have been dispatched triggers recovery in the same
task. A Relay rejection while the agent is offline or not ready preserves
that eligibility if it arrives before Relay assigns a response ID. The caller
starts a fresh Relay session with the current message and up to 16 KiB of
completed text turns, checking recent turns first
and skipping any that exceed the remaining budget. It saves the recovery input
and the start of a new submission window so restarts reuse both. A terminal
rejection that confirms the request never ran permits a new key within the
current window. The initial window for new queued messages starts at dispatch.
When Relay supplies a response ID, the caller polls that ID even if the
submission body is unreadable. An ambiguous submission remains unconfirmed
if its response is missing or its submission window expires. Polling stops after
three invalid response bodies without a valid response in between and reports an
unconfirmed outcome. The user should verify execution before resending.

State defaults to the CSGClaw data directory's `relay/` subdirectory; `--state`
overrides it. A process lock prevents sharing one state file. Files have `0600`
permissions and contain result text; protect them as local conversation data.
Do not delete the state or lock file while running. Up to 1024 records are kept,
reclaiming ACKed history first. Relay-side idempotency owns longer-lived history.
State I/O failure stops the executor. Up to 1024 local session tips are retained;
eviction rejects affected continuations instead of using another context.

The versioned journal migrates legacy ACKed records conservatively; old sessions
have no confirmed checkpoint, so continuation requires a fresh Relay session.
Legacy unacknowledged records lack the original ownership identity and cause an
explicit startup error without modifying the state file. They cannot be safely
replayed by a new instance. Preserve the file for diagnosis and confirm the old
request outcomes before retiring it; do not delete it to retry the same work.

Cancellation waits for the local Session API's runtime cleanup. If stopping
cannot be confirmed within 30 seconds, the result is `cancellation_unconfirmed`
and the executor stays unready. Check local execution before restarting it.

```bash
go test -race ./internal/relay ./cli/relay ./cli/completion ./cli
go test ./...
```

See the [Chinese walkthrough](relay.zh.md) for detailed examples.

## Community plaza discovery

`GET /api/v1/relay/agents` queries the community instance API with
`registered=true&kind=runtime` under the current StarHub login. The public tab
uses the returned `public` flag. The Mine tab shows the caller's registered
agents, both public and private, using `owned_by_current_user` derived from
StarHub's `editable` flag. The query follows pagination and includes both
`platform` and `external` sources. It accepts registered `csgclaw` and
`csgclaw-workspace` runtimes and skips missing/invalid `relay.agent_id` values.
Directory metadata shows the community agent name as its source and maps instance
`source=platform` to `Agentichub`, `source=external` to `CSGClaw` as the publishing
product. Relay publisher IDs remain protocol identities, not product labels.
Each registration is probed through Relay. Community `is_running=true` is not an
online signal for external workers; confirmed offline or missing registrations produce offline entries. Authentication,
rate-limit, transport and server errors fail discovery instead of claiming offline.

Existing cloud conversations pass both `agent_id` and `connection_id` to the
same endpoint. The request probes only the matching registration and returns an
empty list if the registration or connection reference is unavailable.

The browser stores display metadata, Relay ID, URL and a login-scoped connection
reference without tokens. Cloud invocation is authorized against the current
community list, not just this server's own publications. My agents and the task
picker show only saved references matching the current directory's Relay ID,
URL and login-scoped connection reference. Registered offline agents remain
visible but cannot start tasks. Missing entries and failed directory loads are
not reconstructed from cached metadata; saved membership survives temporary
outages and is restored when the same registration returns. Add/remove updates
all mounted consumers and other tabs; remove only cancels the local membership,
never unpublishes or deletes the community instance. The prototype v2 cache is
discarded on upgrade to v3, including old mock registrations; users must re-add
previously saved agents. The local catalog is retained only for standalone
protocol diagnostics.

## Cloud agent chat

In My agents, select a summoned cloud agent and click **Start chat**. The inline
chat sends text to `POST /api/v1/relay/agents/{id}/responses` with the saved
`connection_id`, `input`, a unique `key` and optional `previous_response_id`.
The server resolves credentials and the destination from its own publication
manifest; browser-supplied URLs/tokens are rejected. Responses are returned when
execution finishes. This UI does not stream token deltas.

Unconfirmed submissions with a known response ID return HTTP 202; the browser
queries `GET /api/v1/relay/agents/{id}/responses/{response_id}?connection_id=...`.
Retries reuse the original key. Pending requests with no response ID are not
resubmitted automatically after ten minutes, keeping them within the default
Relay idempotency window. Chat history and pending references are stored locally
per connection and remote agent. Closing/hiding chat does not cancel remote
execution; reopening resumes the pending request. **New conversation** clears
the local response chain, and is disabled while waiting. Relay continuation
records can expire; start a new conversation when an old response is unavailable.

Chat supports text messages and completed replies. The ordinary local task list,
file attachments, remote cancellation controls and token-by-token streaming are
not part of this entry point.


## Cloud tasks

The new-task target picker includes summoned cloud agents with cloud and online/offline
labels. Offline targets cannot be selected. Cloud tasks support text only and do not
require a local model, provider login or workspace configuration. Each task stores its
server-owned connection reference and remote agent ID in a separate IM conversation.
Messages are persisted before background Relay execution; replies appear in the normal
task history, and subsequent messages continue the same response chain. Different tasks
have separate chains. Stable message IDs provide idempotency. Startup resumes pending
messages by querying known response IDs; uncertain submissions older than ten minutes
are never automatically redispatched.


Cloud tasks remain working while submitted results are unconfirmed. Temporary network
or server failures retry the original key or query the saved response ID instead of
recording a terminal failure. Pending work survives server shutdown and restart, and
blocks conversation deletion, archival and history clearing until resolved. If an old
conversation can no longer continue, the UI explains how to start another task and
provides a new-task action that preserves the cloud target.

## Automatic publication in sandbox images

Set `TEMPLATE_ID` and `CSGCLAW_AGENT_AUTO_PUBLISH=true` at container startup.
The image bootstrap passes `--auto-publish=true` to `participant create --bind create`;
both `csgclaw` and `csgclaw-cli` support the flag. The default is false. This applies
only to the startup template agent; false does not unpublish existing registrations.

`LoadSiteCredentials` prefers an authenticated `CurrentSiteCredentials` login,
then the existing managed `CSGHUB_API_BASE_URL` / `CSGHUB_USER_TOKEN` environment
(`CSGHUB_ACCESS_TOKEN` is also supported). Corrupt login-store errors are surfaced;
absent/expired logins may fall back. Without a login UUID, managed publication
ownership is locally bound to a fingerprint of the injected credential. Unpublish
before replacing that credential. The fingerprint is never sent as a trusted user
identity header; OpenCSG still authenticates and injects the actual user UUID.

The server persists intent before remote publication and waits for local runtime
readiness. `POST /api/v1/agents/{id}/relay` with `{"auto_publish":true}` returns 202;
`publishing` indicates queued work, `published` confirms registration/writeback.
Pending work resumes on restart, uses idempotent external registration or a persisted
platform content target, and is cancelled
by DELETE. The image reconciles publication on new creation, marker recovery, and
existing-agent recovery before enabling readiness on port 8888. On bounded polling
timeout it follows the existing bootstrap shutdown path while preserving progress.
Mount `/home/picoclaw/.csgclaw` to retain identity and recovery state.

The deployed community contract is documented in
[StarHub Agent Relay API](https://git-devops.opencsg.com/product/starhub/starhub-server/-/blob/feat-agent-register/.specs/agent-register/api_ee.md).
Local protocol fixtures validate the contract; deployment-specific license,
phone verification, balance admission, proxy and NATS behavior require live integration.

## Publication recovery and deletion

A failed first relay registration does not bind an otherwise unused installation
to that login. Confirmed relay registrations keep their login binding until
cleanup completes. Old empty bindings from earlier versions are cleared on startup.

If an external instance creation commits but its response is lost, unpublish
finds the external instance by its relay/content ID before deleting it. Lookup or
delete failures retain a durable cleanup marker, prevent reconnect, and allow a
later retry. Platform instance deletion remains forbidden.

Publication status exposes `publishing`, `pending`, `removing`, and
`can_unpublish`. The management dialog offers cancellation/cleanup for incomplete
registrations and retry for pending community synchronization. Cancelling a queue
with no remote registration requires no login or network access. Local agent
deletion returns `relay_unpublish_required` until publication/queue/cleanup state
has been removed; deletion is serialized with publication operations.

Community availability probes use at most eight concurrent requests. A timed-out
or cancelled probe returns a directory error instead of a false offline result.
Starting a cloud task probes only its selected community registration.

Pending community synchronization is retried independently of WebSocket
reconnections, with backoff from 1 to 15 seconds. A healthy socket therefore does
not prevent recovery after a community outage. Unpublish cancels and joins the
retry worker before deleting registrations. Platform targets survive unpublish
for republishing the same agent, and are released after successful local agent
deletion so a replacement agent can register against that sandbox instance.

Successful local deletion persists a retired agent ID under the publication
operation lock. Both queued and manual publication check this marker after
acquiring the same lock, rejecting requests that passed an API existence check
before deletion. The marker survives restart; a new ID may reuse the display
name and released sandbox target. Failed local deletion does not retire the ID.

Restored publications wait for credentials with cancellable backoff from 1 to 15
seconds. Missing/expired credentials do not discard the executor; signing back
into the original account resumes registration and the WebSocket connection.
Another account cannot take over the saved publication.

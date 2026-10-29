# Turn diagnostics

The Web UI exposes Turn diagnostics as an icon alongside copy and thread reply, in activity details, and through the conversation log button.
Group conversations can inspect each Agent turn independently.
Errors offer an inline disclosure backed by the same diagnostic record.

## Timing contract

Each turn records message receipt (T0), native request write (T1), native terminal receipt (T2), and transcript delivery completion (T3).
The three headline durations are T0-T1, T1-T2, and T2-T3.
Codex binds terminal notifications to the accepted native turn ID before assigning T2.
DSH records T2 in the ACP response reader, before handing the response to its consumer.
These measurements use the server's monotonic clock and include transport overhead.
Runtime duration includes model calls, tools, callbacks, and user waits; it is not pure Runtime or LLM compute time.
Preparation includes Channel and Engine admission queues, input preparation, and Runtime session setup.
Post-Runtime duration includes event consumption, result processing, persistence, and publication.
CSGClaw spans are reported separately even when they occur inside the Runtime interval.
Overlapping spans use interval union for wall-clock occupancy and are never summed into the parent duration.
The duration ranking partitions the complete wall-clock interval into measured owners, overlapping work, and unattributed Runtime time, so its values sum to the turn total.
All recorded spans and progress events are numbered by start time before filtering.
Progress delivery, native session persistence, turn scopes, and hook events are hidden by default through a multi-select menu beside the owner filter.
The menu explains each category and shows its count and union duration within the current owner filter.
The combined hidden count and interval union remain visible beside the menu, without changing global event numbers or the duration breakdown.
Low-level native HTTP request wrappers are an optional hidden category.
Hidden failed, canceled, interrupted, or at-least-100-ms events produce an inspection hint with a one-click action to reveal their categories; inclusive turn scopes are excluded from the duration threshold and combined hidden duration, while their count and individual coverage remain available.
LLM calls and tools each have chronological call numbers shared by the duration ranking, timeline labels, and event details; filtering never renumbers them.
Hover or focus reveals local timestamps with the timezone offset with millisecond precision, commands, working directories, redacted arguments, and measurement provenance.
The start offset relative to turn receipt appears beside each event name and in hover details.
Clicking a row keeps the same details available for inspection.
Codex model HTTP requests are associated using their exact native thread and turn metadata.
The existing LLM bridge measures each upstream request through response-body consumption, including retries and streaming errors.
The first-response metric records the first response data, not the first model token.
Requests without reliable native identity, including current DSH requests and WebSocket sessions, remain explicitly unmeasured.

Codex exports native OpenTelemetry traces to a per-process loopback HTTP receiver, with a random unguessable path and bounded request bodies.
The receiver associates native turn IDs and trace IDs with the original diagnostic record, including asynchronous exports after the turn has finished.
It admits only known preparation, dispatch, request, stream, tool-wait, persistence, and turn-boundary spans; it never copies arbitrary telemetry attributes, prompts, reasoning, or tool output.
Exports are batched every second and may require refreshing a recently completed diagnostic.
Nested native observations explain only gaps outside bridge, tool, and CSGClaw measurements in the duration distribution.
Their raw inclusive intervals remain visible in the timeline and must not be added together.
A native HTTP request span covers request establishment; the separate stream-receipt phase includes response handling and is not pure model inference or CPU time.
Uncovered intervals remain unattributed; absent native telemetry and historical records are not reconstructed from guesses.

Browser rendering measurements use the submitting document's monotonic clock and are associated with the individual Agent turn.
They are not subtracted from server timestamps.
First-visible-text browser observations use IntersectionObserver and a post-paint observation of actual response text or commentary, excluding loading placeholders, reasoning, and tool status.
Browser observations are not rendered as standalone timeline markers.
An existing server event is not labeled as first-visible-text without an explicit, reliable association to the browser observation.
Older browser first-render measurements are never relabeled as first-visible-text.
Reloading or hiding the submitting document can leave browser measurements unavailable.
Historical messages without diagnostic references do not acquire fabricated timings.

## Storage and privacy

The IM service owns the diagnostic store under its data directory's `diagnostics` subdirectory.
Each turn is saved as an atomic JSON snapshot, with bounded background writes coalescing updates.
Completed records are retained for at most seven days, subject to a 256 MiB disk budget and a 2,000-record bound.
The store caps each timeline at 2,048 spans and diagnostic text at 4 KiB.
Progress-event details use at most half of the span budget, preserving room for model and tool timings.
Tool display metadata has an additional 256 KiB budget per turn.
Timeline truncation, interrupted restarts, and persistence failures are visibly marked incomplete.
Deleting a room or clearing its messages removes its diagnostics.
Graceful server shutdown drains pending writes for up to two seconds.

Model prompts, generated content, and tool outputs are kept separately from the lightweight timeline in compressed, incremental context journals.
Tool names, commands, working directories and bounded, recursively redacted arguments are recorded for diagnosis.
Known sensitive keys and content-bearing fields are omitted from argument previews.
Older records can recover tool display fields from the exact matching turn transcript without inventing timings.
Error messages are limited and redact recognized credentials, bearer tokens, API keys, and URL credentials before storage.
The copy and export actions use the same dynamically assembled full diagnostic bundle.
The original Agent runtime log remains a separate existing diagnostic surface.

## HTTP API

`GET /api/v1/rooms/{id}/diagnostics` returns summaries, supports `source_id`, `thread_id`, `agent_id`, `status`, `cursor`, and `limit`, and defaults to 50 records with a maximum of 100.
`GET /api/v1/rooms/{id}/diagnostics/{diagnostic_id}` returns the detailed timeline and sanitized failure.
`POST /api/v1/rooms/{id}/diagnostics/timings` accepts the source and turn IDs plus browser-relative first and completion durations in milliseconds.
The API uses the existing desktop authentication policy, checks room identity, and rejects Runtime Agent callers.
Client-reported browser durations do not change the authoritative server result.

## Verification

Boundary tests inject time before native invocation and after native terminal receipt, reject stale Codex terminal notifications, and delay ACP response consumption.
Store and API tests cover restart, capacity limits, persistence failure, credential redaction, room isolation, pagination, clearing, and Agent-specific browser timings.
Native Codex and DSH diagnostics cases run against deterministic local model endpoints using `CSGCLAW_TEST_CODEX_BINARY` and `CSGCLAW_TEST_DSH_BINARY`.
The browser fixture uses real HTTP, IM, Channel scheduling, Engine, and transcript delivery, with a deterministic delayed Runtime replacing the external dependency.

## Complete diagnostic bundles

The fixed action bar provides Agent logs, Copy diagnostics, and Export diagnostics with matching button styles.
The selected CSGClaw turn ID remains visible while viewing logs.
`GET /api/v1/rooms/{id}/diagnostics/{diagnostic_id}/bundle` streams a schema-versioned JSON document with execution-time environment snapshots, the selected turn, explicitly related turns, context journals, exact server/runtime logs, messages, task relations and approvals, file metadata, text attachment content, current environment information, and collection issues.
`download=1` changes only the download response header to `csgclaw-turn-{id}-diagnostics.json`.
The filename ID omits the turn ID's existing `turn-csgclaw-` prefix.
Both browser actions use that document, with two-space indentation and `Cache-Control: no-store`.
The optional `timezone` query records the exporting browser's timezone; its user agent is read from the HTTP request.
Runtime Agent credentials cannot read diagnostic bundles.
Related turns are restricted to the same room and linked through source messages or explicit task parent/child identities, never by Agent identity or nearby timestamps.

The Engine records actual runtime input, configuration, ordered progress events, results, and available text file snapshots.
Codex additionally records native RPCs, callbacks, raw notifications before tool preview truncation, runtime configuration, startup handshake information, and explicitly named runtime instruction/config files observed at turn start.
File observations and current server state are labeled separately from actual runtime/model input evidence.
DSH records ACP progress and its execution configuration; model requests without exact native identity are explicitly reported as unavailable rather than assigned to a recent Agent turn.
The model bridge captures original runtime requests, actual upstream requests and attempts, original upstream responses, and responses returned to the runtime, including protocol translation and SSE consumption.
WebSocket collection requires per-message native identity in client metadata and associates subsequent frames through response IDs.
Concurrent WebSocket responses without an unambiguous correlation are marked incomplete instead of being assigned by connection identity.
Binary data URIs and binary attachments are represented by metadata rather than embedded payloads.
Individual model events and text files exceeding 128 MiB are marked incomplete; capture queue pressure or storage failures never fail the user execution.

Complete context uses an independent seven-day, 1 GiB compressed storage budget.
Structured server logs use another seven-day, 1 GiB budget and preserve a literal `[turn_id={turn_id}]` prefix in ordinary server log output.
Foreground, daemon, and desktop launch paths install the same logger.
Active turns are protected from capacity eviction; rejected writes are reported as collection issues.
Context writes are incremental gzip members, and bundle export decodes one context entry or log record at a time.
Room deletion and message clearing also delete context journals, including queued writes racing cleanup.
System logs include only non-turn records in each included turn's receipt-to-completion interval extended by one minute on either side, capped at export time.
Runtime stderr includes exact turn matches and explicitly labeled context from the same process instance and time window.
Complete field matching prevents IDs such as `turn-1` from matching `turn-10`, and lines carrying other turn IDs are excluded.
A per-process marker, PID and byte boundaries prevent a restarted process that appends to the same file from being mistaken for the original process.

Complete-context redaction preserves long text while recursively removing credential fields, authentication headers, cookies, recognized key formats, private key blocks, and configured model credentials found inside arbitrary model text.
It does not use the timeline's 4 KiB string truncation.
Credential-bearing command-line options are redacted in both `--token value` and `--api-key=value` forms while preserving argument order and public options.
Embedded JSON and JSON configuration files retain exact numeric tokens, including tool argument IDs above 2^53 and high-precision decimals.
Historical uncaptured context, expired or unavailable files/logs, incomplete streams, and running executions are listed in `collection.issues`.
A partial bundle remains downloadable and the UI shows a brief notice.

## Failure evidence and collection coverage

The model HTTP ingress records a generated request ID, declared content length, actual bytes read, retained bytes, the byte limit, hash scope, read completion and HTTP failure status.
Oversized headers, chunked overflow, interrupted reads and malformed JSON are recorded before model forwarding.
Malformed JSON includes its parser offset without retaining a potentially sensitive malformed prefix.
Forwarding failures retain the valid original request and link it to the ingress request ID.
Recoverable publication failures capture their exact turn, runtime ID, operation stage, path, error type, OS error code and underlying cause without changing whether the model may retry.

Each turn exports a section coverage manifest in `collection.sections`.
The manifest distinguishes `complete`, `partial`, `in_progress` and `not_applicable`, with stable reasons and expected/observed counts where meaningful.
Every observed model request and upstream attempt must have response and terminal evidence or an explicit error; observing one successful request cannot hide a missing later exchange.
Old records lack the new evidence version and cannot claim the new sections were collected.
An applicable section that is absent, an unreadable or rotated runtime log, a missing known server log shard, a collector restart gap, or a failed/truncated read makes collection incomplete.
`collection.complete` describes the required turn evidence sections, not all possible application or operating-system diagnostics.

## Persistence and delivery measurements

The built-in transcript path carries the turn's diagnostic context through synchronous and delayed progress delivery.
Measurements include progress-lock wait/hold, IM-lock wait/hold, state snapshot, normalization, serialization, comparison reads, writes, file synchronization, rename, cleanup, attachment preparation and publication.
`diagnostic.performance` aggregates count, total duration, maximum duration and relevant byte/item counters, while `performance.operation` entries preserve operation-level observations and persistence mode.
Zero write or sync counts are valid for unchanged files, transient progress and memory-only services.
A record without measured operations is marked missing rather than presented as zero-cost persistence.
Lock-hold and persistence durations are inclusive of their child work and must not be added together as disjoint wall-clock durations.
Queue observations and execution-admission load counts describe concurrency without including unrelated conversation content.
Instrumentation preserves the existing lock ownership, synchronous writes, atomic replacement, file permissions, message publication order and unchanged-file optimization.

## Runtime process evidence

The Codex and DSH process observers bind active and recently completed turns to a specific runtime instance.
Process exit records include PID, exit code, runtime identity and cleanup outcome; Codex decoder failures are recorded before pending requests are closed.
Shared process events are labeled as process evidence and do not claim a unique turn caused the error.
Timestamped nearby stderr, including MCP and transport errors without turn IDs, is labeled `runtime_process_window`.
Untimed evidence within known turn byte boundaries is labeled `runtime_process_byte_range`, with timestamp uncertainty reported explicitly.
The server log collector persists observed coverage intervals and file sizes so a quiet interval can be distinguished from an unobserved interval or a missing shard after restart.
No standalone application diagnostic bundle is added by this feature.

Failures that reach HTTP before the native `turn/start` response are buffered briefly under the exact Agent/session/turn identity and attached when that identity binds.
This bounded failure buffer preserves the observation timestamp, never guesses the latest Agent turn, and marks dropped evidence when capacity is exceeded.

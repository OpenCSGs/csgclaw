# Turn diagnostics

The Web UI exposes Performance diagnostics as an icon alongside copy and thread reply, in activity details, and through the conversation log button.
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

Model prompts, generated content, and tool outputs are not copied into diagnostic records.
Tool names, commands, working directories and bounded, recursively redacted arguments are recorded for diagnosis.
Known sensitive keys and content-bearing fields are omitted from argument previews.
Older records can recover tool display fields from the exact matching turn transcript without inventing timings.
Error messages are limited and redact recognized credentials, bearer tokens, API keys, and URL credentials before storage.
The copy and export actions use the same sanitized snapshot.
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

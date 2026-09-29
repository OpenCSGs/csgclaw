# Turn progress

The Web transcript presents each Agent Engine turn as a process disclosure followed by the streamed answer.
The process text, including runtime-provided reasoning, is expanded by default while running and automatically collapses when execution finishes.
Finished turns also load collapsed and can be reopened manually.
The reader can collapse the complete process manually; tool groups do not add another disclosure.
Tool groups show all aligned, numbered command rows separated by subtle rules.
Command numbers continue across commentary and reasoning sections within one turn and start again at one in the next turn.
Each row independently reveals its full command, output and compact execution metadata without adding another indentation level.
The transcript is the only vertical scroll owner, including when the pointer is over a command or its output.
Running turns place context usage and the stop control together beside the elapsed time, outside the process disclosure.
Hovering or focusing the context statistics pauses transcript auto-follow until inspection ends; deliberate scrolling still works normally.
Controls bind to the authoritative work lease using request, sender, room and thread identity and disappear when the turn ends.
The composer retains controls until the corresponding message is available, or when a running thread is not currently visible.
Blocking questions and approvals remain visible outside it.

## Data ownership

The runtime adapter preserves `TurnEvent.item_id` and `TurnEvent.phase` for assistant text.
`TurnEvent.text_snapshot` replaces the matching item when native completion corrects an earlier draft.
Codex commentary is delivered as progress and is excluded from the Engine's final output.
Unclassified text is provisional: a subsequent tool start moves it into the process, and the last text after tools becomes the answer.
If execution ends with a tool, the most recent provisional text remains visible as the answer.
Explicit commentary is never promoted merely because the runtime omitted a final answer.

The Web channel renderer owns a `csgclaw.turn_progress` metadata snapshot on the stable final message ID.
The snapshot carries turn identity, revision, lifecycle timestamps, status and ordered process items.
Concurrent turns remain isolated by the existing complete channel/Agent/conversation/turn identity.
Tools update in place by their native call ID.
Tool labels derive from structured command actions or an explicit tool-kind mapping; unknown calls use generic labels.
Shell source is not parsed to infer an operation.

Text and bursts of tool updates are coalesced into 50ms publication windows.
In-memory IM snapshots and the existing SSE message stream expose running updates without a disk write per token.
Tool boundaries, text boundaries and terminal states request persistence; adjacent tool boundaries share the next publication window.
A pending timer cannot overwrite terminal delivery.
Progress and drafts are activity records and cannot wake another agent; only the successful final response uses normal final delivery.
Image-only retries retain the existing image-message lifecycle.

The transcript, thread views and activity inspector read the same snapshot.
Clients ignore stale revisions and refresh snapshots on reconnect.
Persisted running snapshots become interrupted when loaded into a new service, with the clock stopped at the last saved update.
History refreshes within the running service retain newer in-memory progress, including updates that have not reached the next persistence checkpoint.
A process crash can lose the uncheckpointed tail of a text segment.
There is no historical migration or independent event database.

## Verification

Go integration tests cover live IM projection, terminal ordering, cancellation, failures, unclassified text, output bursts and restart recovery.
Frontend tests cover grouping, revision ordering, default text visibility, individual command disclosure, manual inspection and independent answer visibility.
The opt-in `TestTurnProgressBrowserFixture` uses a real Engine, Web renderer, HTTP/SSE and compiled UI with deterministic runtime events.
Set `CSGCLAW_PROGRESS_READY_FILE` to an output path to launch that fixture for headless browser verification.
The fixture exposes test-only controls for starting, releasing and canceling turns and stops through `/__e2e/finish`.

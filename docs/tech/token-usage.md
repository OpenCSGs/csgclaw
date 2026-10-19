# Per-turn token consumption

English | [中文](token-usage.zh.md)

Completed conversation and thread replies show the selected model and total
reported token consumption. A selected model ID of `auto` hides the model name,
including the actual upstream routing target. Hovering or focusing the count
shows exact input/output counters and available cache/reasoning subsets.

One turn includes all model steps needed for that Agent's reply, including tool
round trips. Input includes the context actually sent on every request: system
instructions, history, tools and tool results. Repeated history therefore counts
again. Output can include reasoning and tool-call arguments, not just visible
answer text. Cached input and reasoning output are subsets, never extra tokens
added on top. This is consumption, not a monetary cost calculation.

## Accounting sources

- Codex reports `thread/tokenUsage/updated` with `tokenUsage.total` (thread
  lifetime) and `tokenUsage.last` (one model step). The adapter subtracts the
  admitted turn's baseline, replacing snapshots instead of adding duplicate
  events. For a cold restored thread, the first total minus last establishes
  the historical baseline. A context-recovery continuation keeps the original
  baseline. Background memory sessions are excluded from user reply delivery.
  The HTTP LLM Bridge also checks each successful upstream attempt's final
  usage before protocol conversion. A missing or invalid usage marks that exact
  native Agent/thread/turn as partial, even when other calls returned counts.
  Auth/protocol rejection retries are excluded. The flag survives later valid
  counters and recovery continuations, and resets for the next user turn.
- DSH's managed context bridge forwards committed `assistant/message` and
  `compaction/summary` usage, including automatic summarization model calls.
  The adapter sums distinct event sequence IDs inside the active session turn.
  Missing or interrupted usage marks the turn as partial. It does not reuse
  ACP `usage_update.used`, which measures context occupancy.
- Consumption travels as `token_usage` activity events and is persisted in
  `metadata.csgclaw.turn_progress.usage` with the selected model snapshot.
  Unknown usage is distinct from a reported zero. Failed, canceled and saved
  interrupted turns keep available accounting and mark it partial. Old replies
  without consumption metadata are left unchanged.

## Wire formats verified on OpenCSG

On 2026-10-19, authenticated minimal requests to the configured OpenCSG gateway
returned the following counters. Request IDs, content and credentials are not
stored here. Counts differ between calls; these are samples, not constants.

Chat Completions, `auto`, streaming with `stream_options.include_usage=true`:

```json
{"usage":{"prompt_tokens":14,"completion_tokens":144,"total_tokens":158,"completion_tokens_details":{"reasoning_tokens":140,"text_tokens":144},"prompt_tokens_details":{"cached_tokens":0,"text_tokens":14}}}
```

The response model was `qwen3.7-plus`; the UI still hides it because the selected
model was `auto`. A separately selected `qwen3.7-plus` also returned usage.
Non-streaming Chat responses returned JSON with the same counter names and gzip
content encoding. Streaming usage may arrive after `finish_reason`, in an empty
`choices` chunk. Both protocol conversion and text fallback preserve that chunk.

Responses, `auto`, streaming terminal response:

```json
{"usage":{"input_tokens":50,"output_tokens":139,"total_tokens":189,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":135},"x_details":[{"input_tokens":50,"output_tokens":139,"total_tokens":189}]}}
```

Non-streaming Responses was also verified. `x_details` repeats accounting for
billing and is not added to the top-level counters.

Messages, `auto`, streaming:

```json
{"type":"message_start","message":{"usage":{"input_tokens":0,"output_tokens":0}}}
{"type":"message_delta","usage":{"input_tokens":14,"output_tokens":164}}
```

The later input counter replaces the initial zero. Standard Anthropic cache read
and creation counters are separate from uncached input; normalized aggregate
input includes them once.

The configured local CSGHub Lite endpoint was unavailable during these probes.
Other vendors without configured credentials were not called. Compatibility is
verified by the three protocol formats, not a claim of live testing every vendor.

## Verification

`internal/llm/token_usage_test.go` covers all three protocols, streaming and JSON
callers, both Chat and Responses consumers, and late usage-only chunks. Native
Codex protocol switching verifies consumption through an actual Codex process:

```sh
CSGCLAW_TEST_CODEX_BINARY=/absolute/path/to/codex go test ./internal/llm -run TestBundledCodexProtocolSwitchE2E -count=1
CSGCLAW_TEST_CODEX_BINARY=/absolute/path/to/codex go test ./internal/api -run TestNativeCodexUsageCompletenessThroughBridge -count=1
CSGCLAW_TEST_DSH_BINARY=/absolute/path/to/dsh go test ./internal/runtime/dsh -run 'TestContextUsageNativeDSHE2E/(usage|auto_compact|cold_resume)$' -count=1
```

Runtime and delivery tests cover distinct model steps, duplicate/stale events,
turn boundaries, partial accounting, and history persistence. Frontend tests
cover total formatting, unavailable/zero usage and hiding the auto model.
The UI Lab includes normal, auto and long-model-name reply footers.

The pinned DSH `0.1.5-rc.3` native test was also run: each fixture turn reported
1,000 input + 20 output = 1,020 tokens, independently of approximately 12.5K
estimated context occupancy, including after a cold session restore.
Automatic compaction was also exercised: turns issuing two model calls report
2,040 tokens, counting both the reply and its summary. Native Codex HTTP tests
cover complete usage, a missing first or last call, all calls missing usage,
and an independent complete next turn.

# Model context management

CSGClaw resolves model context capacity per provider and model.
Codex and DSH consume this metadata; this feature does not change OpenClaw configuration.

## Configuration

The existing provider APIs return `model_metadata`, keyed by model ID, with `context_window` and `context_source`.
The read-only `model_defaults` map supplies automatic capacities for reset previews before saving.
Provider updates accept `model_overrides`, keyed by model ID, with an optional `context_window` field.
An empty override map restores automatic settings for all models of that provider.
A zero or omitted context override restores automatic capacity for that model.
Provider refresh retains user overrides, and changing the endpoint invalidates discovered metadata.

Resolution precedence is user override, provider deployment metadata, the common-model name catalog, and the default.
The unknown-model capacity is 200,000 tokens.
The catalog matches common GPT, Claude, DeepSeek, Qwen, GLM, Gemini, Kimi, MiniMax, MiMo and OpenCSG Agentic names across providers, including namespaces, punctuation, dates and serving suffixes.
These are reference capacities; provider-reported limits and user overrides take precedence, and unrecognized versions retain the default.
Catalog sources are recorded alongside the entries in `internal/modelcap/catalog.go`.
Provider extensions `context_window` and `context_length` are supported; their presence is not assumed for standard OpenAI model lists.

Settings are persisted with the existing provider state and apply to all referencing agents.
Affected Codex and DSH agents are marked as requiring configuration application, then the existing lifecycle gate drains active work before restarting the runtime without deleting its history.
A failed or timed-out restart retains the pending flag and existing conversation.

## Runtime behavior

Codex receives the capacity and a 75 percent auto-compaction threshold in its managed catalog and configuration.
Its installed protocol does not expose a reliable off switch, so the UI describes automatic compaction as enabled without offering a nonfunctional toggle.
On a recognized context overflow, CSGClaw waits for the failed native turn, compacts once, then continues with empty input only if no assistant output or tool execution has occurred.
The user message is never appended a second time.

DSH receives `contextWindow` through its native model settings.
Its managed compaction patch sets `thresholdRatio: 0.75` and `maxOverflowRetries: 1`.
The Agent runtime option `auto_compact` accepts `enabled` (default) or `disabled`.
The small managed ACP bridge forwards DSH's existing compaction lifecycle events; it does not implement a second compaction mechanism.

## Context usage

The existing participant work status carries `context_usage` separately from tools and thoughts.
Snapshots are isolated by native session and business turn and use the same revision-controlled status delivery and reconnect snapshots.
Codex contributes the latest request's token usage rather than its cumulative total; DSH contributes ACP `used` and `size`.
The runtime's actual usable capacity takes precedence over the configured capacity for the indicator.
Before runtime reporting, usage is unknown rather than zero.
A completed compaction invalidates the old occupancy until a fresh reading is available.

Agent model selection forms do not display context metadata.
The provider list shows compact model rows with editable K/M token capacities, pending-change badges, refresh feedback and save progress.
UI units use 1K = 1,000 tokens and 1M = 1,000,000 tokens; token counts are not byte sizes.
Each row refreshes provider discovery with progress and success or failure feedback while preserving user overrides.
Restore automatic settings is available inside the capacity editor.
Image generation models show their purpose instead of a default chat window.
The image generation request does not consume this context setting; image size limits belong to the image service and delivery path.
Editing preserves the exact token count and the API continues to store integer tokens.

The 16-pixel ring appears between the current tool activity and Stop, supports hover and keyboard focus, and follows the activity row's lifecycle.
Its tooltip shows usage, remaining tokens, capacity, source, model, automatic-compaction state, report time, and estimated-data status.
Values over capacity retain their real percentage in the tooltip while the ring saturates at 100 percent.

## Verification

Run regular backend and frontend suites, plus the opt-in real-process tests with isolated local model fixtures:

```sh
CSGCLAW_TEST_CODEX_BINARY="$PWD/bin/codex" go test ./internal/runtime/codex -run TestContextUsageBundledCodexE2E -count=1
CSGCLAW_TEST_DSH_BINARY=/absolute/path/to/dsh go test ./internal/runtime/dsh -run TestContextUsageNativeDSHE2E -count=1
```

No real model credentials are needed for these fixtures.
A single oversized input, non-reducible history, or failed summary can still exceed the model capacity; no chat history is silently deleted to hide that failure.

Thread start and resume explicitly set the same capacity and compaction threshold, overriding settings retained in older threads.
Codex provider errors never trigger Chat Completions fallback or poison the Responses capability cache.

# Session API Demo Frontend Guide

The standalone demo is available at `/#/session-demo` when CSGClaw is running.
It is a live client for the Agent Session API rather than a simulated chat surface.

## Purpose

The page demonstrates the smallest professional frontend boundary for creating or reusing an agent session.
It intentionally stays outside the main workspace shell while sharing the existing React providers, design tokens, UI primitives, localization, and Vite build.

The demo covers these behaviors:

- It loads running Codex agents supported by the Phase 1 Session endpoint.
- It generates a new session ID with `crypto.randomUUID()` or accepts a path-safe existing session ID.
- It calls `POST /api/v1/agents/{agent}/sessions/{session_id}/responses` with `stream: true` and renders text deltas as they arrive.
- It displays a deterministic conversation label before the first request.
- It explains that Session execution uses Agent Engine directly and creates no IM Room or Message.
- It disables overlapping sends and lets the user explicitly cancel the active Runtime turn.
- It stores a bounded browser-local transcript for up to 20 sessions and 100 messages per session.

## Source Boundaries

The source is split by responsibility so a production frontend or a test mock can replace one layer at a time.

- `src/api/agentSessions.ts` owns live HTTP requests, SSE parsing, and OpenAI-style error extraction.
- `src/models/agentSessions.ts` owns the wire types, conversation-label formatting, and session validation.
- `src/pages/SessionDemoPage/sessionDemoStorage.ts` owns the transcript limits and storage normalization.
- `src/pages/SessionDemoPage/useSessionDemo.ts` owns the page state machine and exports the `SessionDemoTransport` seam.
- `src/pages/SessionDemoPage/SessionDemoPage.tsx` owns accessible rendering and user interactions.
- `src/pages/SessionDemoPage/SessionDemoPage.module.css` owns the responsive page presentation.

`SessionDemoTransport` is the mock boundary:

```ts
export type SessionDemoTransport = {
  listAgents: () => Promise<SessionAgent[]>;
  streamResponse: (
    request: CreateAgentSessionResponseRequest,
    onTextDelta?: (delta: string) => void,
  ) => Promise<AgentSessionStreamResult>;
  cancelResponse: (request: CancelAgentSessionResponseRequest) => Promise<void>;
};
```

A deterministic component test can pass a fixture implementation to `useSessionDemo` without intercepting global `fetch`.
A production client can implement the same interface with a generated SDK, a gateway, or another request library.

## Session And Transcript Behavior

The URL query parameters are `agent` and `session`, for example `/#/session-demo?agent=reviewer&session=review-01`.
The demo uses the stable agent name in both its page URL and Agent Session API request path, while its local transcript record keeps the canonical Agent ID.
Explicit URL values take precedence over the last saved selection.

The server owns the runtime conversation context.
The demo stores only its visible successful user and assistant messages in `csgclaw.session-demo.v1` localStorage.
Reusing a server session from another browser continues the agent context but starts with an empty visible transcript.
Streaming text is transient UI state and is persisted only after `message_stop` completes the turn.

The agent selector locks after the first successful turn because a global session cannot change agents.
The New Session action creates a fresh client ID, clears visible state, and unlocks agent selection.
The Runtime conversation is created or resumed lazily when the first message is sent.

## Error And Cancellation Behavior

The transport extracts `error.message` and `error.code` from the API envelope while retaining the HTTP status.
Failed and canceled inputs return to the composer so the user can edit or retry them.
Canceling calls `POST /api/v1/agents/{agent}/sessions/{session_id}/responses/cancel`, which cancels the Engine context and closes the browser stream after Runtime cleanup.
The browser aborts its stream only if the explicit cancellation request fails.
The composer remains disabled until the server confirms Runtime cancellation and turn cleanup, preventing a follow-up from racing the canceled turn.

## Production Hardening

The demo is intended for the trusted local `csgclaw serve` boundary.
A remotely exposed production application should add authentication, authorization, rate limiting, origin controls, audit retention policy, and an explicit trust model for acting as admin.

The v1 API streams text and tool events but has no server transcript read endpoint, attachments, or session ownership token.
A production frontend should add those capabilities at the API boundary rather than inferring them from localStorage.

# Session API Demo Frontend Guide

The standalone demo is available at `/#/session-demo` when CSGClaw is running.
It is a live client for the Agent Session API rather than a simulated chat surface.

## Purpose

The page demonstrates the smallest professional frontend boundary for creating or reusing an agent session.
It intentionally stays outside the main workspace shell while sharing the existing React providers, design tokens, UI primitives, localization, and Vite build.

The demo covers these behaviors:

- It loads agents that have exactly the local active participant required by the session endpoint.
- It generates a new session ID with `crypto.randomUUID()` or accepts a path-safe existing session ID.
- It calls `POST /api/v1/agents/{agent}/sessions/{session_id}/responses` and renders final `output_text`.
- It displays the deterministic audit room title before the first request.
- It explains that Anonymous is persisted as `user-admin` and that notify-all remains enabled.
- It disables overlapping sends and lets the user cancel the active HTTP request.
- It stores a bounded browser-local transcript for up to 20 sessions and 100 messages per session.

## Source Boundaries

The source is split by responsibility so a production frontend or a test mock can replace one layer at a time.

- `src/api/agentSessions.ts` owns live HTTP requests and OpenAI-style error extraction.
- `src/models/agentSessions.ts` owns the wire types, room-title formatting, and session validation.
- `src/pages/SessionDemoPage/sessionDemoStorage.ts` owns the transcript limits and storage normalization.
- `src/pages/SessionDemoPage/useSessionDemo.ts` owns the page state machine and exports the `SessionDemoTransport` seam.
- `src/pages/SessionDemoPage/SessionDemoPage.tsx` owns accessible rendering and user interactions.
- `src/pages/SessionDemoPage/SessionDemoPage.module.css` owns the responsive page presentation.

`SessionDemoTransport` is the mock boundary:

```ts
export type SessionDemoTransport = {
  listAgents: () => Promise<SessionAgent[]>;
  createResponse: (request: CreateAgentSessionResponseRequest) => Promise<AgentSessionResponse>;
};
```

A deterministic component test can pass a fixture implementation to `useSessionDemo` without intercepting global `fetch`.
A production client can implement the same interface with a generated SDK, a gateway, or another request library.

## Session And Transcript Behavior

The URL query parameters are `agent` and `session`, for example `/#/session-demo?agent=reviewer&session=review-01`.
The demo uses the stable agent name in both its page URL and Agent Session API request path, while persisted room metadata keeps the canonical agent ID.
Explicit URL values take precedence over the last saved selection.

The server owns the room history and the runtime conversation context.
The demo stores only its visible successful user and assistant messages in `csgclaw.session-demo.v1` localStorage.
Reusing a server session from another browser continues the agent context but starts with an empty visible transcript.

The agent selector locks after the first successful turn because a global session cannot change agents.
The New Session action creates a fresh client ID, clears visible state, and unlocks agent selection.
The room itself is created lazily when the first message is sent.

## Error And Cancellation Behavior

The transport extracts `error.message` and `error.code` from the API envelope while retaining the HTTP status.
Failed and canceled inputs return to the composer so the user can edit or retry them.
Canceling aborts the browser request, while the server requests runtime cancellation and keeps the session busy until the turn closes or its cleanup grace period expires.

## Production Hardening

The demo is intended for the trusted local `csgclaw serve` boundary.
A remotely exposed production application should add authentication, authorization, rate limiting, origin controls, audit retention policy, and an explicit trust model for acting as admin.

The v1 API has no streaming output, server transcript read endpoint, attachments, or session ownership token.
A production frontend should add those capabilities at the API boundary rather than inferring them from localStorage.

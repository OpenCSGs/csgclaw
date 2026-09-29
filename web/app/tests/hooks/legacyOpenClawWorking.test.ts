import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentLike } from "@/models/agents";
import { buildUsersById, type IMConversation, type IMMessage } from "@/models/conversations";
import {
  useLegacyOpenClawWorkingFallback,
  withLegacyOpenClawWorkingFallback,
} from "@/hooks/workspace/legacyOpenClawWorking";

const now = Date.parse("2026-07-15T12:00:00Z");
const usersById = buildUsersById([
  { id: "user-admin", name: "Admin" },
  { id: "user-worker", name: "Worker" },
]);

function conversation(messages: IMMessage[]): IMConversation {
  return {
    id: "room-1",
    is_direct: true,
    members: ["user-admin", "user-worker"],
    messages,
  };
}

function agent(runtimeKind: string): AgentLike {
  return { id: "agent-worker", name: "OpenClaw Worker", runtime_kind: runtimeKind, user_id: "user-worker" };
}

function fallback(messages: IMMessage[], runtimeKind = "openclaw_sandbox") {
  return withLegacyOpenClawWorkingFallback({
    agents: [agent(runtimeKind)],
    authoritative: [],
    conversation: conversation(messages),
    currentUserID: "user-admin",
    hasObservedWorkLease: () => false,
    now,
    usersById,
  });
}

function pendingParticipant(
  activityAfter = "2026-07-15T11:59:30Z",
  requestID: string | null = "message-1",
  name = "OpenClaw Worker",
) {
  return {
    activityAfter,
    id: "user-worker",
    name,
    ...(requestID ? { requestID } : {}),
  };
}

function toolActivity(status: string): IMMessage {
  return {
    content: JSON.stringify({
      content: {
        body: `Tool ${status}: exec`,
        msgtype: "com.opencsg.csgclaw.agent.tool",
        tool: { id: "tool-1", kind: "execute", status, title: "Run command" },
      },
      event_id: `tool-${status}`,
      sender: "user-worker",
      type: "com.opencsg.csgclaw.agent.activity",
      version: 1,
    }),
    created_at: "2026-07-15T11:59:30Z",
    id: `tool-${status}`,
    sender_id: "user-worker",
  };
}

function questionActivity(status: string, resolvedAt?: string): IMMessage {
  return {
    content: JSON.stringify({
      content: {
        body: `Question ${status}`,
        msgtype: "com.opencsg.csgclaw.agent.question",
        question: {
          id: "question-1",
          questions: [{ header: "Choice", id: "choice", options: [], question: "Choose" }],
          requested_at: "2026-07-15T11:57:00Z",
          resolved_at: resolvedAt,
          status,
        },
      },
      event_id: `question-${status}`,
      sender: "user-worker",
      type: "com.opencsg.csgclaw.agent.activity",
      version: 1,
    }),
    created_at: resolvedAt ?? "2026-07-15T11:57:00Z",
    id: `question-${status}`,
    sender_id: "user-worker",
  };
}

afterEach(() => {
  vi.useRealTimers();
});

describe("withLegacyOpenClawWorkingFallback", () => {
  it("keeps recent unanswered OpenClaw messages compatible", () => {
    expect(
      fallback([
        {
          content: "hello",
          created_at: "2026-07-15T11:59:30Z",
          id: "message-1",
          sender_id: "user-admin",
        },
      ]),
    ).toEqual([pendingParticipant()]);
  });

  it("does not enable the fallback for other runtimes", () => {
    expect(
      fallback(
        [{ content: "hello", created_at: "2026-07-15T11:59:30Z", id: "message-1", sender_id: "user-admin" }],
        "picoclaw_sandbox",
      ),
    ).toEqual([]);
  });

  it("bounds pending-message inference and clears it on a normal reply", () => {
    expect(
      fallback([{ content: "old", created_at: "2026-07-15T11:57:00Z", id: "message-old", sender_id: "user-admin" }]),
    ).toEqual([]);
    expect(
      fallback([
        { content: "hello", created_at: "2026-07-15T11:59:30Z", id: "message-1", sender_id: "user-admin" },
        { content: "done", created_at: "2026-07-15T11:59:40Z", id: "message-2", sender_id: "user-worker" },
      ]),
    ).toEqual([]);
  });

  it("expires the compatibility result without waiting for another realtime event", () => {
    vi.useFakeTimers();
    vi.setSystemTime(now);
    const pendingConversation = conversation([
      {
        content: "hello",
        created_at: "2026-07-15T11:59:30Z",
        id: "message-1",
        sender_id: "user-admin",
      },
    ]);
    const { result } = renderHook(() =>
      useLegacyOpenClawWorkingFallback({
        agents: [agent("openclaw_sandbox")],
        authoritative: [],
        conversation: pendingConversation,
        currentUserID: "user-admin",
        hasObservedWorkLease: () => false,
        usersById,
      }),
    );

    expect(result.current).toEqual([pendingParticipant()]);
    act(() => vi.advanceTimersByTime(90_000));
    expect(result.current).toEqual([]);
  });

  it("tracks legacy tool activity until its terminal event", () => {
    expect(fallback([toolActivity("running")])).toEqual([pendingParticipant("2026-07-15T11:59:30.000Z", null)]);
    expect(fallback([toolActivity("running"), toolActivity("completed")])).toEqual([]);
    expect(fallback([{ ...toolActivity("running"), created_at: "2026-07-15T11:57:00Z" }])).toEqual([]);
  });

  it("keeps answered questions working until the follow-up arrives", () => {
    const userMessage: IMMessage = {
      content: "hello",
      created_at: "2026-07-15T11:57:00Z",
      id: "message-1",
      sender_id: "user-admin",
    };
    const answered = questionActivity("answered", "2026-07-15T11:59:50Z");

    expect(fallback([userMessage, answered])).toEqual([pendingParticipant("2026-07-15T11:57:00Z")]);
    expect(
      fallback([
        userMessage,
        answered,
        {
          content: "Thanks for your answer.",
          created_at: "2026-07-15T11:59:55Z",
          id: "message-2",
          sender_id: "user-worker",
        },
      ]),
    ).toEqual([]);
  });

  it("does not keep interrupted questions working", () => {
    expect(
      fallback([
        {
          content: "hello",
          created_at: "2026-07-15T11:59:30Z",
          id: "message-1",
          sender_id: "user-admin",
        },
        questionActivity("interrupted", "2026-07-15T11:59:50Z"),
      ]),
    ).toEqual([]);
  });

  it("clears legacy message and tool inference when a new conversation starts", () => {
    const newConversation: IMMessage = {
      content: '<slash-command name="new" arg="conversation"></slash-command>',
      created_at: "2026-07-15T11:59:55Z",
      id: "message-new",
      sender_id: "user-admin",
    };

    expect(
      fallback([
        {
          content: "hello",
          created_at: "2026-07-15T11:59:30Z",
          id: "message-1",
          sender_id: "user-admin",
        },
        newConversation,
      ]),
    ).toEqual([]);
    expect(fallback([toolActivity("running"), newConversation])).toEqual([]);
  });

  it("deduplicates the compatibility result with an authoritative lease", () => {
    const result = withLegacyOpenClawWorkingFallback({
      agents: [agent("openclaw_sandbox")],
      authoritative: [{ id: "user-worker", name: "Lease Worker" }],
      conversation: conversation([
        { content: "hello", created_at: "2026-07-15T11:59:30Z", id: "message-1", sender_id: "user-admin" },
      ]),
      currentUserID: "user-admin",
      hasObservedWorkLease: () => false,
      now,
      usersById,
    });

    expect(result).toEqual([pendingParticipant("2026-07-15T11:59:30Z", "message-1", "Lease Worker")]);
  });

  it("stops legacy inference after a participant has emitted a real lease", () => {
    const result = withLegacyOpenClawWorkingFallback({
      agents: [agent("openclaw_sandbox")],
      authoritative: [],
      conversation: conversation([
        { content: "hello", created_at: "2026-07-15T11:59:30Z", id: "message-1", sender_id: "user-admin" },
      ]),
      currentUserID: "user-admin",
      hasObservedWorkLease: (participantID) => participantID === "user-worker",
      now,
      usersById,
    });

    expect(result).toEqual([]);
  });
  it("preserves separate authoritative requests for the same agent", () => {
    const leases = [
      { id: "user-worker", name: "Worker", leaseID: "lease-a", requestID: "request-a", roomID: "room-1" },
      {
        id: "user-worker",
        name: "Worker",
        leaseID: "lease-b",
        requestID: "request-b",
        roomID: "room-1",
        threadRootID: "thread",
      },
    ];
    const result = withLegacyOpenClawWorkingFallback({
      agents: [agent("codex")],
      authoritative: leases,
      conversation: conversation([]),
      currentUserID: "user-admin",
      hasObservedWorkLease: () => true,
      now,
      usersById,
    });
    expect(result.map((item) => item.leaseID)).toEqual(["lease-a", "lease-b"]);
    expect(result.map((item) => item.requestID)).toEqual(["request-a", "request-b"]);
  });
});

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentLike } from "@/models/agents";
import type { IMServerEvent } from "@/models/conversations";
import { useParticipantWorkStatus } from "@/hooks/workspace/useParticipantWorkStatus";

const startedAt = Date.parse("2026-07-14T12:00:00Z");

function workEvent(overrides: Partial<NonNullable<IMServerEvent["work"]>> = {}): IMServerEvent {
  return {
    type: "participant.work.updated",
    work: {
      expires_at: "2026-07-14T12:00:15Z",
      kind: "agent_turn",
      lease_id: "lease-1",
      participant_id: "pt-worker",
      reason: "started",
      registry_epoch: "epoch-1",
      request_id: "message-1",
      revision: 1,
      room_id: "room-1",
      state: "working",
      user_id: "user-worker",
      ...overrides,
    },
  };
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("useParticipantWorkStatus", () => {
  it("projects active lease state and expires it through one deadline timer", () => {
    vi.useFakeTimers();
    vi.setSystemTime(startedAt);
    const { result } = renderHook(() =>
      useParticipantWorkStatus({
        agents: [{ id: "agent-worker", name: "Agent Worker", user_id: "user-worker" } as AgentLike],
        users: [{ id: "user-worker", name: "Roster Worker" }],
      }),
    );

    expect(result.current.workingParticipantsForRoom("room-1")).toEqual([]);
    expect(result.current.hasObservedWorkLease("user-worker")).toBe(false);

    act(() =>
      result.current.handleRealtimeEvent(
        workEvent({
          capabilities: ["thinking_status_v1", "turn_stop_v1", "work_stage_v1"],
          reason: "status_updated",
          revision: 2,
          status: { phase: "thinking", sequence: 1, stage: "preparing_reply" },
        }),
      ),
    );
    expect(result.current.workingParticipantsForRoom("room-1")).toEqual([
      expect.objectContaining({
        id: "user-worker",
        leaseID: "lease-1",
        name: "Agent Worker",
        participantID: "pt-worker",
        requestID: "message-1",
        roomID: "room-1",
        workStage: "preparing_reply",
      }),
    ]);
    expect(result.current.hasObservedWorkLease("pt-worker")).toBe(true);
    expect(result.current.hasObservedWorkLease("user-worker")).toBe(true);
    expect(vi.getTimerCount()).toBe(1);

    act(() =>
      result.current.handleRealtimeEvent(
        workEvent({
          capabilities: ["turn_stop_v1"],
          reason: "stop_requested",
          revision: 3,
          stop_requested_at: "2026-07-14T12:00:01Z",
          stop_state: "stop_requested",
        }),
      ),
    );
    expect(result.current.workingParticipantsForRoom("room-1")[0]).toEqual(
      expect.objectContaining({ canStop: true, stopping: true }),
    );

    act(() =>
      result.current.handleRealtimeEvent(
        workEvent({
          capabilities: ["turn_stop_v1"],
          reason: "stop_failed",
          revision: 4,
          stop_error: "turn control is unavailable",
          stop_requested_at: "2026-07-14T12:00:01Z",
          stop_state: "stop_failed",
        }),
      ),
    );
    expect(result.current.workingParticipantsForRoom("room-1")[0]).toEqual(
      expect.objectContaining({
        canStop: true,
        stopError: "turn control is unavailable",
        stopping: false,
      }),
    );

    act(() => vi.advanceTimersByTime(15_000));
    expect(result.current.workingParticipantsForRoom("room-1")).toEqual([]);
    expect(vi.getTimerCount()).toBe(1);

    act(() =>
      result.current.handleRealtimeEvent({
        participant: { id: "pt-worker" },
        type: "participant.updated",
      }),
    );
    expect(result.current.workingParticipantsForRoom("room-1")).toEqual([]);
    expect(result.current.hasObservedWorkLease("pt-worker")).toBe(false);
  });

  it("does not display idle work updates", () => {
    vi.useFakeTimers();
    vi.setSystemTime(startedAt);
    const { result } = renderHook(() =>
      useParticipantWorkStatus({
        agents: [],
        users: [{ id: "user-worker", name: "Worker" }],
      }),
    );

    act(() => result.current.handleRealtimeEvent(workEvent({ reason: "released", revision: 2, state: "idle" })));
    expect(result.current.workingParticipantsForRoom("room-1")).toEqual([]);
  });

  it("posts a stop request for the exact participant lease", async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            accepted: true,
            lease_id: "lease-2",
            participant_id: "pt-worker",
            registry_epoch: "epoch-1",
            request_id: "message-2",
            requested_at: "2026-07-20T03:00:08Z",
            room_id: "room-1",
            state: "stop_requested",
          }),
          { status: 202 },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);
    const { result } = renderHook(() => useParticipantWorkStatus({ agents: [], users: [] }));

    await act(async () => {
      await result.current.stopWorkingTurn({
        id: "user-worker",
        leaseID: "lease-2",
        name: "Worker",
        participantID: "pt-worker",
        requestID: "message-2",
        roomID: "room-1",
      });
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "api/v1/channels/csgclaw/participants/pt-worker/work:stop",
      expect.objectContaining({
        body: JSON.stringify({
          lease_id: "lease-2",
          request_id: "message-2",
          room_id: "room-1",
        }),
        method: "POST",
      }),
    );
  });
});

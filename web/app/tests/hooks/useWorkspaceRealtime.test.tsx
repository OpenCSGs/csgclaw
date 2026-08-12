import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { useWorkspaceRealtime } from "@/hooks/workspace/useWorkspaceRealtime";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { buildUsersById, type IMData, type IMServerEvent } from "@/models/conversations";
import type { AgentLike } from "@/models/agents";

const subscribeIMEventsMock = vi.fn();

vi.mock("@/shared/realtime/imEvents", () => ({
  subscribeIMEvents: (handler: (payload: IMServerEvent) => void) => subscribeIMEventsMock(handler),
}));

const bootstrapData: IMData = {
  current_user_id: "u-admin",
  rooms: [],
  users: [
    { id: "u-admin", name: "admin" },
    { id: "u-worker", name: "worker" },
  ],
};

function renderWorkspaceRealtime(
  options: {
    agents?: AgentLike[];
    onAgentTurnNotificationEvent?: (event: IMServerEvent) => void;
    onRefreshAgentState?: (agentID: string) => Promise<AgentLike | null>;
    queryClient?: QueryClient;
    refreshWorkspaceAgents?: (options?: { silent?: boolean }) => Promise<AgentLike[]>;
    refreshWorkspaceBootstrap?: () => Promise<IMData | null>;
    setBootstrapData?: (value: IMData | null | ((current: IMData | null) => IMData | null)) => void;
  } = {},
) {
  const agents: AgentLike[] = options.agents ?? [];
  const setBootstrapData =
    options.setBootstrapData ??
    vi.fn((value: IMData | null | ((current: IMData | null) => IMData | null)) => {
      if (typeof value === "function") {
        value(bootstrapData);
      }
    });
  const queryClient = options.queryClient ?? createQueryClient();

  return renderHook(
    () =>
      useWorkspaceRealtime({
        agents,
        onAgentTurnNotificationEvent: options.onAgentTurnNotificationEvent,
        onConversationEvent: vi.fn(),
        onFloatingConversationEvent: vi.fn(),
        onRefreshAgentState: options.onRefreshAgentState ?? vi.fn(async () => null),
        onUpgradeStatusChange: vi.fn(),
        refreshWorkspaceAgents: options.refreshWorkspaceAgents ?? vi.fn(async () => []),
        refreshWorkspaceBootstrap: options.refreshWorkspaceBootstrap ?? vi.fn(async () => null),
        setBootstrapData,
        usersById: buildUsersById(bootstrapData.users),
      }),
    { wrapper: createWrapper(queryClient) },
  );
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

describe("useWorkspaceRealtime", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    subscribeIMEventsMock.mockReset();
    subscribeIMEventsMock.mockReturnValue(() => {});
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("subscribes once and debounces participant roster refreshes", async () => {
    let eventHandler: ((payload: IMServerEvent) => void) | null = null;
    subscribeIMEventsMock.mockImplementation((handler: (payload: IMServerEvent) => void) => {
      eventHandler = handler;
      return () => {};
    });
    const refreshWorkspaceAgents = vi.fn(async () => []);
    const refreshWorkspaceBootstrap = vi.fn(async () => null);
    const setBootstrapData = vi.fn((value: IMData | null | ((current: IMData | null) => IMData | null)) => {
      if (typeof value === "function") {
        value(bootstrapData);
      }
    });

    renderWorkspaceRealtime({ refreshWorkspaceAgents, refreshWorkspaceBootstrap, setBootstrapData });

    expect(subscribeIMEventsMock).toHaveBeenCalledTimes(1);

    act(() => {
      eventHandler?.({
        participant: { id: "pt-worker", type: "agent" },
        type: "participant.created",
      });
    });

    expect(setBootstrapData).toHaveBeenCalledTimes(1);
    expect(refreshWorkspaceAgents).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(100);
    });

    expect(refreshWorkspaceBootstrap).toHaveBeenCalledTimes(1);
    expect(refreshWorkspaceAgents).toHaveBeenCalledWith({ silent: true });
  });

  it("invalidates teams when a team lifecycle event arrives", () => {
    let eventHandler: ((payload: IMServerEvent) => void) | null = null;
    subscribeIMEventsMock.mockImplementation((handler: (payload: IMServerEvent) => void) => {
      eventHandler = handler;
      return () => {};
    });
    const queryClient = createQueryClient();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");

    renderWorkspaceRealtime({ queryClient });

    act(() => {
      eventHandler?.({
        team: {
          id: "team-weather",
          title: "Weather team",
          lead_agent_id: "u-manager",
          member_agent_ids: ["u-weather"],
          status: "active",
          created_at: "",
          updated_at: "",
        },
        team_id: "team-weather",
        type: "team.created",
      });
    });

    expect(invalidateSpy).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(100);
    });

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: workspaceQueryKeys.teams() });
  });

  it("refreshes non-running agent state when that agent sends a message", () => {
    let eventHandler: ((payload: IMServerEvent) => void) | null = null;
    subscribeIMEventsMock.mockImplementation((handler: (payload: IMServerEvent) => void) => {
      eventHandler = handler;
      return () => {};
    });
    const onRefreshAgentState = vi.fn(async () => null);

    renderWorkspaceRealtime({
      agents: [
        {
          id: "u-worker",
          name: "worker",
          role: "worker",
          runtime_kind: "picoclaw_sandbox",
          status: "unknown",
        },
      ],
      onRefreshAgentState,
    });

    act(() => {
      eventHandler?.({
        message: {
          content: "reply",
          sender_id: "u-worker",
        },
        room_id: "room-1",
        type: "message.created",
      });
    });

    expect(onRefreshAgentState).toHaveBeenCalledWith("u-worker");
  });

  it("forwards realtime events to the turn notification controller", () => {
    let eventHandler: ((payload: IMServerEvent) => void) | null = null;
    subscribeIMEventsMock.mockImplementation((handler: (payload: IMServerEvent) => void) => {
      eventHandler = handler;
      return () => {};
    });
    const onAgentTurnNotificationEvent = vi.fn();
    renderWorkspaceRealtime({ onAgentTurnNotificationEvent });
    const event: IMServerEvent = {
      type: "participant.work.updated",
      work: {
        expires_at: "2026-08-11T12:00:15Z",
        kind: "agent_turn",
        lease_id: "lease-1",
        participant_id: "pt-worker",
        reason: "released",
        registry_epoch: "epoch-1",
        request_id: "message-1",
        revision: 2,
        room_id: "room-1",
        state: "idle",
        user_id: "user-worker",
      },
    };

    act(() => eventHandler?.(event));

    expect(onAgentTurnNotificationEvent).toHaveBeenCalledWith(event);
  });
});

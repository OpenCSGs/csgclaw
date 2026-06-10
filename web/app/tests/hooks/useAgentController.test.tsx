import { useRef, useState, type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import {
  fetchAgent,
  fetchAgentProfile,
  fetchAgentProfileModels,
  fetchAgentWorkspace,
  runAgentActionRequest,
  updateAgentRequest,
} from "@/api/agents";
import { createTeamRequest, fetchTeams } from "@/api/tasks";
import { useAgentController } from "@/hooks/workspace/useAgentController";
import { WorkspacePaneTypes } from "@/models/routing";
import type { WorkspacePane } from "@/models/routing";
import type { AgentLike, AgentProfileLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";

vi.mock("react-router-dom", async () => {
  const actual = await vi.importActual<typeof import("react-router-dom")>("react-router-dom");
  return {
    ...actual,
    useBlocker: () => ({
      proceed: vi.fn(),
      reset: vi.fn(),
      state: "unblocked",
    }),
  };
});

vi.mock("@/api/tasks", async () => {
  const actual = await vi.importActual<typeof import("@/api/tasks")>("@/api/tasks");
  return {
    ...actual,
    createTeamRequest: vi.fn(),
    fetchTeams: vi.fn(async () => []),
  };
});

vi.mock("@/api/agents", async () => {
  const actual = await vi.importActual<typeof import("@/api/agents")>("@/api/agents");
  return {
    ...actual,
    fetchAgent: vi.fn(),
    fetchAgentProfile: vi.fn(),
    fetchAgentProfileModels: vi.fn(),
    fetchAgentWorkspace: vi.fn(),
    runAgentActionRequest: vi.fn(),
    updateAgentRequest: vi.fn(),
  };
});

const oldImage = "registry.example/opencsghq/picoclaw:2026.5.27";
const actionImage = "registry.example/opencsghq/picoclaw:2026.6.1";
const latestImage = "registry.example/opencsghq/picoclaw:2026.6.8";

const oldAgent: AgentLike = {
  agent_profile: {
    image_upgrade_required: true,
    model_id: "gpt-test",
    profile_complete: true,
    provider: "codex",
  },
  id: "u-manager",
  image: oldImage,
  model_id: "gpt-test",
  name: "manager",
  profile_complete: true,
  provider: "codex",
  role: "manager",
  runtime_kind: "picoclaw_sandbox",
  status: "running",
};

const latestAgent: AgentLike = {
  ...oldAgent,
  agent_profile: {
    ...oldAgent.agent_profile,
    image_upgrade_required: false,
  },
  image: latestImage,
};

const profile: AgentProfileLike = {
  image_upgrade_required: false,
  model_id: "gpt-test",
  profile_complete: true,
  provider: "codex",
};

const incompleteProfile: AgentProfileLike = {
  image_upgrade_required: false,
  model_id: "",
  profile_complete: false,
  provider: "csghub_lite",
};

const t: TranslateFn = (key) => key;

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function useAgentControllerHarness(
  options: { activePane?: WorkspacePane; managerProfile?: AgentProfileLike | null } = {},
) {
  const [agents, setAgents] = useState<AgentLike[]>([oldAgent]);
  const refreshWorkspaceAgents = vi.fn(async () => [oldAgent]);
  const selectAgentRef = useRef(vi.fn());
  const selectAgent = selectAgentRef.current;

  const controller = useAgentController({
    activeConversationId: "",
    activePane: options.activePane ?? { type: WorkspacePaneTypes.agent, id: "u-manager" },
    agents,
    agentsLoaded: true,
    agentsQuery: {
      data: agents,
      error: null,
      isError: false,
      isFetched: true,
    } as UseQueryResult<AgentLike[]>,
    bootstrapConfig: null,
    data: null,
    hubTemplates: [],
    localRuntimeImages: [],
    locale: "en",
    managerProfile: options.managerProfile ?? null,
    refreshHubTemplates: vi.fn(async () => undefined),
    refreshWorkspaceAgents,
    refreshWorkspaceBootstrap: vi.fn(async () => null),
    refreshWorkspaceBootstrapConfig: vi.fn(async () => null),
    refreshWorkspaceManagerProfile: vi.fn(async () => null),
    rooms: [],
    selectAgent,
    selectComputer: vi.fn(),
    selectConversation: vi.fn(),
    selectHub: vi.fn(),
    setAgentsData: (value: AgentLike[] | ((current: AgentLike[]) => AgentLike[])) => {
      setAgents((current) => (typeof value === "function" ? value(current) : value));
    },
    setSelectedHubTemplateId: vi.fn(),
    t,
  });

  return { controller, selectAgent };
}

describe("useAgentController", () => {
  beforeEach(() => {
    vi.mocked(fetchAgent).mockReset();
    vi.mocked(fetchAgentProfile).mockReset();
    vi.mocked(fetchAgentProfileModels).mockReset();
    vi.mocked(fetchAgentWorkspace).mockReset();
    vi.mocked(createTeamRequest).mockReset();
    vi.mocked(fetchTeams).mockReset();
    vi.mocked(runAgentActionRequest).mockReset();
    vi.mocked(updateAgentRequest).mockReset();
    vi.mocked(fetchAgent).mockResolvedValueOnce(oldAgent).mockResolvedValueOnce(latestAgent);
    vi.mocked(fetchAgentProfile).mockResolvedValue(profile);
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: [] });
    vi.mocked(fetchAgentWorkspace).mockResolvedValue({ entries: [] });
    vi.mocked(createTeamRequest).mockResolvedValue({
      channel: "csgclaw",
      created_at: "2026-06-10T00:00:00Z",
      id: "team-1",
      lead_agent_id: "u-manager",
      room_id: "room-1",
      status: "active",
      title: "Untitled Team",
      updated_at: "2026-06-10T00:00:00Z",
    });
    vi.mocked(fetchTeams).mockResolvedValue([]);
    vi.mocked(runAgentActionRequest).mockResolvedValue({
      ...oldAgent,
      image: actionImage,
    });
    vi.mocked(updateAgentRequest).mockResolvedValue(latestAgent);
  });

  it("refreshes the selected agent detail from a cache-busted agent fetch after upgrade", async () => {
    const { result } = renderHook(() => useAgentControllerHarness().controller, { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.agentViewProps.draft?.image).toBe(oldImage));
    await waitFor(() => expect(fetchAgentWorkspace).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.agentViewProps.onUpgrade?.(oldAgent);
    });

    await waitFor(() => expect(result.current.agentViewProps.item?.image).toBe(latestImage));
    await waitFor(() => expect(fetchAgentWorkspace).toHaveBeenCalledTimes(2));
    expect(result.current.agentViewProps.draft?.image).toBe(latestImage);
    expect(result.current.agentViewProps.savedDraft?.image).toBe(latestImage);
    expect(runAgentActionRequest).toHaveBeenCalledWith("u-manager", "upgrade");
    expect(fetchAgent).toHaveBeenLastCalledWith("u-manager", { cacheBust: true });
  });

  it("refreshes the selected agent workspace after saving manager profile changes", async () => {
    const { result } = renderHook(() => useAgentControllerHarness().controller, { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.agentViewProps.draft?.image).toBe(oldImage));
    await waitFor(() => expect(fetchAgentWorkspace).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.agentViewProps.onSave?.();
    });

    await waitFor(() => expect(fetchAgentWorkspace).toHaveBeenCalledTimes(2));
    expect(updateAgentRequest).toHaveBeenCalledWith(
      "u-manager",
      expect.objectContaining({
        name: "manager",
      }),
    );
  });

  it("routes incomplete manager profile setup to the manager agent page", async () => {
    const { result } = renderHook(
      () =>
        useAgentControllerHarness({
          activePane: { type: WorkspacePaneTypes.conversation, id: "room-1" },
          managerProfile: incompleteProfile,
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() =>
      expect(result.current.selectAgent).toHaveBeenCalledWith({ id: "u-manager" }, { replace: true }),
    );
    expect(result.current.controller.agentViewProps.notice).toBe("profileIncompleteRedirectNotice");
    expect("managerProfileSetupModalProps" in result.current.controller).toBe(false);
  });

  it("clears the manager setup redirect notice after a short timeout", async () => {
    vi.useFakeTimers();
    try {
      const { result } = renderHook(
        () =>
          useAgentControllerHarness({
            activePane: { type: WorkspacePaneTypes.conversation, id: "room-1" },
            managerProfile: incompleteProfile,
          }),
        { wrapper: createWrapper() },
      );

      await act(async () => {
        await Promise.resolve();
      });

      expect(result.current.controller.agentViewProps.notice).toBe("profileIncompleteRedirectNotice");

      await act(async () => {
        vi.advanceTimersByTime(5000);
        await Promise.resolve();
      });

      expect(result.current.controller.agentViewProps.notice).toBe("");
    } finally {
      vi.useRealTimers();
    }
  });

  it("creates teams with agent id fields from the agent list", async () => {
    const { result } = renderHook(() => useAgentControllerHarness(), { wrapper: createWrapper() });

    act(() => {
      result.current.controller.openCreateTeamModal();
    });

    await waitFor(() => expect(result.current.controller.createTeamModalProps?.teamMemberIDs).toEqual(["u-manager"]));

    await act(async () => {
      await result.current.controller.createTeamModalProps?.onCreate();
    });

    expect(createTeamRequest).toHaveBeenCalledWith({
      channel: "csgclaw",
      lead_agent_id: "u-manager",
      member_agent_ids: ["u-manager"],
      title: "teamNewFallbackTitle",
    });
  });
});

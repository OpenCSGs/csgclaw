import { useEffect, useRef, useState, type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import {
  batchAddAgentSkillsRequest,
  deleteAgentSkillRequest,
  fetchAgent,
  fetchAgentProfile,
  fetchAgentProfileDefaults,
  fetchAgentProfileModels,
  fetchAgentSkills,
  fetchAgentSkillsFile,
  fetchAgentWorkspace,
  deleteFeishuParticipantRequest,
  finalizeFeishuRegistrationRequest,
  runAgentActionRequest,
  startFeishuRegistrationRequest,
  updateAgentRequest,
} from "@/api/agents";
import { createUserRequest } from "@/api/im";
import { fetchSkills } from "@/api/skills";
import { createTeamRequest, fetchTeams } from "@/api/tasks";
import { useAgentController } from "@/hooks/workspace/useAgentController";
import { WorkspacePaneTypes } from "@/models/routing";
import type { WorkspacePane } from "@/models/routing";
import type { AgentLike, AgentProfileLike } from "@/models/agents";
import type { IMConversation, IMData, TranslateFn } from "@/models/conversations";
import { AGENT_AVATAR_OPTIONS } from "@/shared/avatarOptions";

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
    batchAddAgentSkillsRequest: vi.fn(),
    deleteAgentSkillRequest: vi.fn(),
    fetchAgent: vi.fn(),
    fetchAgentProfile: vi.fn(),
    fetchAgentProfileDefaults: vi.fn(),
    fetchAgentProfileModels: vi.fn(),
    fetchAgentSkills: vi.fn(),
    fetchAgentSkillsFile: vi.fn(),
    fetchAgentWorkspace: vi.fn(),
    deleteFeishuParticipantRequest: vi.fn(),
    finalizeFeishuRegistrationRequest: vi.fn(),
    runAgentActionRequest: vi.fn(),
    startFeishuRegistrationRequest: vi.fn(),
    updateAgentRequest: vi.fn(),
  };
});

vi.mock("@/api/skills", async () => {
  const actual = await vi.importActual<typeof import("@/api/skills")>("@/api/skills");
  return {
    ...actual,
    fetchSkills: vi.fn(),
  };
});

vi.mock("@/api/im", async () => {
  const actual = await vi.importActual<typeof import("@/api/im")>("@/api/im");
  return {
    ...actual,
    createUserRequest: vi.fn(),
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
  instructions: "reply briefly",
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

const feishuRegistrationStorageKey = "csgclaw.im.feishuRegistrations";

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
  options: {
    activePane?: WorkspacePane;
    agents?: AgentLike[];
    data?: IMData | null;
    managerProfile?: AgentProfileLike | null;
  } = {},
) {
  const [agents, setAgents] = useState<AgentLike[]>(options.agents ?? [oldAgent]);
  const refreshWorkspaceAgentsRef = useRef(vi.fn(async () => options.agents ?? [oldAgent]));
  const refreshWorkspaceBootstrapRef = useRef(vi.fn(async () => null));
  const refreshWorkspaceBootstrapConfigRef = useRef(vi.fn(async () => null));
  const refreshWorkspaceManagerProfileRef = useRef(vi.fn(async () => null));
  const refreshWorkspaceAgents = refreshWorkspaceAgentsRef.current;
  const refreshWorkspaceBootstrap = refreshWorkspaceBootstrapRef.current;
  const refreshWorkspaceBootstrapConfig = refreshWorkspaceBootstrapConfigRef.current;
  const refreshWorkspaceManagerProfile = refreshWorkspaceManagerProfileRef.current;
  const selectAgentRef = useRef(vi.fn());
  const selectAgent = selectAgentRef.current;
  const selectConversationRef = useRef(vi.fn());
  const selectConversation = selectConversationRef.current;
  const data = options.data ?? null;

  useEffect(() => {
    if (options.agents) {
      setAgents(options.agents);
    }
  }, [options.agents]);

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
    data,
    hubTemplates: [],
    locale: "en",
    managerProfile: options.managerProfile ?? null,
    refreshHubTemplates: vi.fn(async () => undefined),
    refreshWorkspaceAgents,
    refreshWorkspaceBootstrap,
    refreshWorkspaceBootstrapConfig,
    refreshWorkspaceManagerProfile,
    rooms: data?.rooms ?? [],
    selectAgent,
    selectComputer: vi.fn(),
    selectConversation,
    selectHub: vi.fn(),
    setAgentsData: (value: AgentLike[] | ((current: AgentLike[]) => AgentLike[])) => {
      setAgents((current) => (typeof value === "function" ? value(current) : value));
    },
    setSelectedHubTemplateId: vi.fn(),
    t,
  });

  return {
    controller,
    refreshWorkspaceAgents,
    refreshWorkspaceBootstrap,
    refreshWorkspaceBootstrapConfig,
    refreshWorkspaceManagerProfile,
    selectAgent,
    selectConversation,
  };
}

describe("useAgentController", () => {
  beforeEach(() => {
    vi.mocked(fetchAgent).mockReset();
    vi.mocked(fetchAgentProfile).mockReset();
    vi.mocked(fetchAgentProfileDefaults).mockReset();
    vi.mocked(fetchAgentProfileModels).mockReset();
    vi.mocked(batchAddAgentSkillsRequest).mockReset();
    vi.mocked(deleteAgentSkillRequest).mockReset();
    vi.mocked(fetchAgentWorkspace).mockReset();
    vi.mocked(createUserRequest).mockReset();
    vi.mocked(fetchAgentSkills).mockReset();
    vi.mocked(fetchAgentSkillsFile).mockReset();
    vi.mocked(fetchSkills).mockReset();
    vi.mocked(deleteFeishuParticipantRequest).mockReset();
    vi.mocked(finalizeFeishuRegistrationRequest).mockReset();
    vi.mocked(createTeamRequest).mockReset();
    vi.mocked(fetchTeams).mockReset();
    vi.mocked(runAgentActionRequest).mockReset();
    vi.mocked(startFeishuRegistrationRequest).mockReset();
    vi.mocked(updateAgentRequest).mockReset();
    window.localStorage.removeItem(feishuRegistrationStorageKey);
    vi.mocked(fetchAgent).mockResolvedValueOnce(oldAgent).mockResolvedValueOnce(latestAgent);
    vi.mocked(fetchAgentProfile).mockResolvedValue(profile);
    vi.mocked(fetchAgentProfileDefaults).mockResolvedValue(profile);
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: [] });
    vi.mocked(batchAddAgentSkillsRequest).mockResolvedValue(undefined);
    vi.mocked(deleteAgentSkillRequest).mockResolvedValue(undefined);
    vi.mocked(fetchAgentWorkspace).mockResolvedValue({ entries: [] });
    vi.mocked(createUserRequest).mockResolvedValue({ id: "u-worker", name: "worker" });
    vi.mocked(fetchAgentSkills).mockResolvedValue({ entries: [] });
    vi.mocked(fetchAgentSkillsFile).mockResolvedValue({ content: "", path: "SKILL.md", size: 0 });
    vi.mocked(fetchSkills).mockResolvedValue([
      { name: "alpha", description: "Alpha skill" },
      { name: "beta", description: "Beta skill" },
    ]);
    vi.mocked(deleteFeishuParticipantRequest).mockResolvedValue(undefined);
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
    vi.mocked(startFeishuRegistrationRequest).mockResolvedValue({
      agent_id: "u-dev",
      connect_url: "https://feishu.example/connect",
      expires_at: "2999-01-01T00:00:00Z",
      next_poll_seconds: 1,
      participant_id: "dev",
      registration_id: "reg-dev",
      status: "started",
    });
    vi.mocked(finalizeFeishuRegistrationRequest).mockResolvedValue({
      agent_id: "u-dev",
      config_saved: true,
      participant_id: "dev",
      status: "configured",
    });
    vi.mocked(updateAgentRequest).mockResolvedValue(latestAgent);
  });

  afterEach(() => {
    window.localStorage.removeItem(feishuRegistrationStorageKey);
  });

  it("refreshes the selected agent detail from a cache-busted agent fetch after upgrade", async () => {
    const { result } = renderHook(() => useAgentControllerHarness().controller, { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.agentViewProps.draft?.image).toBe(oldImage));
    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.agentViewProps.onUpgrade?.(oldAgent);
    });

    await waitFor(() => expect(result.current.agentViewProps.item?.image).toBe(latestImage));
    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledTimes(2));
    expect(result.current.agentViewProps.draft?.image).toBe(latestImage);
    expect(result.current.agentViewProps.savedDraft?.image).toBe(latestImage);
    expect(runAgentActionRequest).toHaveBeenCalledWith("u-manager", "upgrade");
    expect(fetchAgent).toHaveBeenLastCalledWith("u-manager", { cacheBust: true });
  });

  it("refreshes the selected agent workspace after saving manager profile changes", async () => {
    const { result } = renderHook(() => useAgentControllerHarness().controller, { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.agentViewProps.draft?.image).toBe(oldImage));
    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.agentViewProps.onSave?.();
    });

    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledTimes(2));
    expect(updateAgentRequest).toHaveBeenCalledWith(
      "u-manager",
      expect.objectContaining({
        instructions: "reply briefly",
        name: "manager",
      }),
    );
  });

  it("does not wait for bootstrap or skill refresh after saving only the selected agent model", async () => {
    const aiGatewayAgent: AgentLike = {
      agent_profile: {
        model_id: "MiniMax-M2.4",
        model_provider_id: "opencsg-aigateway",
        profile_complete: true,
        provider: "api",
      },
      id: "u-worker",
      image: "worker:latest",
      instructions: "reply briefly",
      model_id: "MiniMax-M2.4",
      model_provider_id: "opencsg-aigateway",
      name: "worker",
      profile_complete: true,
      provider: "api",
      role: "worker",
      runtime_kind: "picoclaw_sandbox",
      status: "running",
    };
    const savedAgent: AgentLike = {
      ...aiGatewayAgent,
      agent_profile: {
        ...aiGatewayAgent.agent_profile,
        model_id: "MiniMax-M2.5",
      },
      model_id: "MiniMax-M2.5",
    };
    vi.mocked(fetchAgent).mockReset();
    vi.mocked(fetchAgent).mockResolvedValueOnce(aiGatewayAgent).mockResolvedValue(savedAgent);
    vi.mocked(fetchAgentProfile).mockResolvedValue(aiGatewayAgent.agent_profile ?? {});
    vi.mocked(updateAgentRequest).mockResolvedValue(savedAgent);

    const { result } = renderHook(
      () =>
        useAgentControllerHarness({
          activePane: { type: WorkspacePaneTypes.agent, id: "u-worker" },
          agents: [aiGatewayAgent],
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(result.current.controller.agentViewProps.draft?.model_id).toBe("MiniMax-M2.4"));
    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledTimes(1));

    act(() => {
      const draft = result.current.controller.agentViewProps.draft;
      result.current.controller.agentViewProps.onDraftChange?.({
        ...draft!,
        model_id: "MiniMax-M2.5",
      });
    });

    await act(async () => {
      await result.current.controller.agentViewProps.onSave?.();
    });

    expect(updateAgentRequest).toHaveBeenCalledWith(
      "u-worker",
      expect.objectContaining({
        agent_profile: expect.objectContaining({
          model_id: "MiniMax-M2.5",
          model_provider_id: "opencsg-aigateway",
        }),
      }),
    );
    expect(result.current.refreshWorkspaceBootstrap).not.toHaveBeenCalled();
    expect(fetchAgentSkills).toHaveBeenCalledTimes(1);
  });

  it("reloads the selected agent draft when the same routed agent gains profile fields and there are no unsaved edits", async () => {
    const partialAgent: AgentLike = {
      id: "u-worker",
      name: "worker",
      role: "worker",
      runtime_kind: "picoclaw_sandbox",
      status: "running",
      image: "worker:latest",
      instructions: "reply briefly",
      profile_complete: false,
    };
    const fullAgent: AgentLike = {
      ...partialAgent,
      agent_profile: {
        provider: "csghub_lite",
        model_provider_id: "csghub-lite",
        model_id: "MiniMax-M2.5",
        reasoning_effort: "medium",
        enable_fast_mode: false,
        profile_complete: true,
      },
      profile: "csghub-lite.MiniMax-M2.5",
      profile_complete: true,
      provider: "csghub_lite",
      model_provider_id: "csghub-lite",
      model_id: "MiniMax-M2.5",
    };

    vi.mocked(fetchAgent).mockReset();
    vi.mocked(fetchAgentProfile).mockReset();
    vi.mocked(fetchAgent)
      .mockRejectedValueOnce(new Error("not ready"))
      .mockResolvedValueOnce(fullAgent)
      .mockResolvedValue(fullAgent);
    vi.mocked(fetchAgentProfile)
      .mockRejectedValueOnce(new Error("not ready"))
      .mockResolvedValue(fullAgent.agent_profile ?? {});

    const { result, rerender } = renderHook(
      ({ agents }) =>
        useAgentControllerHarness({
          activePane: { type: WorkspacePaneTypes.agent, id: "u-worker" },
          agents,
        }).controller,
      {
        initialProps: {
          agents: [partialAgent],
        },
        wrapper: createWrapper(),
      },
    );

    await waitFor(() => expect(result.current.agentViewProps.draft?.model_id).toBe(""));

    rerender({
      agents: [fullAgent],
    });

    await waitFor(() => expect(result.current.agentViewProps.draft?.model_id).toBe("MiniMax-M2.5"));
    await waitFor(() => expect(result.current.agentViewProps.savedDraft?.model_id).toBe("MiniMax-M2.5"));
  });

  it("loads global skill candidates and filters already-installed agent skills", async () => {
    vi.mocked(fetchAgentSkills).mockResolvedValue({
      entries: [
        { name: "alpha", path: "alpha", type: "dir" },
        { name: "SKILL.md", path: "alpha/SKILL.md", type: "file" },
      ],
    });
    vi.mocked(fetchAgentSkillsFile).mockResolvedValue({
      content: "---\ndescription: Alpha skill\n---\n# Alpha\n",
      path: "alpha/SKILL.md",
      size: 32,
    });

    const { result } = renderHook(() => useAgentControllerHarness().controller, { wrapper: createWrapper() });

    await waitFor(() => expect(fetchSkills).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(result.current.agentViewProps.skills).toHaveLength(1));

    expect(result.current.agentViewProps.skillCandidates).toEqual([{ name: "beta", description: "Beta skill" }]);
  });

  it("adds selected global skills into the current agent runtime and refreshes skills", async () => {
    vi.mocked(fetchAgentSkills)
      .mockResolvedValueOnce({ entries: [] })
      .mockResolvedValueOnce({
        entries: [
          { name: "alpha", path: "alpha", type: "dir" },
          { name: "SKILL.md", path: "alpha/SKILL.md", type: "file" },
        ],
      });
    vi.mocked(fetchAgentSkillsFile).mockResolvedValue({
      content: "---\ndescription: Alpha skill\n---\n# Alpha\n",
      path: "alpha/SKILL.md",
      size: 32,
    });

    const { result } = renderHook(() => useAgentControllerHarness().controller, { wrapper: createWrapper() });

    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledTimes(1));

    await act(async () => {
      await result.current.agentViewProps.onAddSkills?.(["alpha"]);
    });

    expect(batchAddAgentSkillsRequest).toHaveBeenCalledWith("u-manager", ["alpha"]);
    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledTimes(2));
    expect(result.current.agentViewProps.skillAddError).toBe("");
    expect(result.current.agentViewProps.skills.map((item) => item.name)).toEqual(["alpha"]);
  });

  it("deletes an agent-scoped skill and refreshes the agent skill list", async () => {
    vi.mocked(fetchAgentSkills)
      .mockResolvedValueOnce({
        entries: [
          { name: "alpha", path: "alpha", type: "dir" },
          { name: "SKILL.md", path: "alpha/SKILL.md", type: "file" },
        ],
      })
      .mockResolvedValueOnce({ entries: [] });
    vi.mocked(fetchAgentSkillsFile).mockResolvedValue({
      content: "---\ndescription: Alpha skill\n---\n# Alpha\n",
      path: "alpha/SKILL.md",
      size: 32,
    });

    const { result } = renderHook(() => useAgentControllerHarness().controller, { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.agentViewProps.skills.map((item) => item.name)).toEqual(["alpha"]));

    await act(async () => {
      await result.current.agentViewProps.onDeleteSkill?.({ name: "alpha" });
    });

    expect(deleteAgentSkillRequest).toHaveBeenCalledWith("u-manager", "alpha");
    await waitFor(() => expect(fetchAgentSkills).toHaveBeenCalledTimes(2));
    expect(result.current.agentViewProps.skillDeleteError).toBe("");
    expect(result.current.agentViewProps.skills).toEqual([]);
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

  it("routes manager direct messages to their conversation page", async () => {
    const directConversation: IMConversation = {
      id: "dm-manager",
      is_direct: true,
      members: ["u-admin", "manager"],
      messages: [],
      title: "manager",
    };
    const { result } = renderHook(
      () =>
        useAgentControllerHarness({
          agents: [oldAgent],
          data: {
            current_user_id: "u-admin",
            rooms: [directConversation],
            users: [
              { id: "u-admin", name: "admin" },
              { id: "manager", name: "manager" },
            ],
          },
        }),
      { wrapper: createWrapper() },
    );

    await act(async () => {
      await result.current.controller.agentViewProps.onOpenDM(oldAgent);
    });

    expect(result.current.selectConversation).toHaveBeenCalledWith("dm-manager", { rooms: [directConversation] });
    expect(createUserRequest).not.toHaveBeenCalled();
  });

  it("routes worker direct messages to their conversation page", async () => {
    const workerAgent: AgentLike = {
      id: "agent-worker",
      name: "worker",
      role: "worker",
      runtime_kind: "picoclaw_sandbox",
      status: "running",
      user_id: "u-worker",
    };
    const directConversation: IMConversation = {
      id: "dm-worker",
      is_direct: true,
      members: ["u-admin", "u-worker"],
      messages: [],
      title: "worker",
    };
    const { result } = renderHook(
      () =>
        useAgentControllerHarness({
          agents: [oldAgent, workerAgent],
          data: {
            current_user_id: "u-admin",
            rooms: [directConversation],
            users: [
              { id: "u-admin", name: "admin" },
              { id: "u-worker", name: "worker" },
            ],
          },
        }),
      { wrapper: createWrapper() },
    );

    await act(async () => {
      await result.current.controller.agentViewProps.onOpenDM(workerAgent);
    });

    expect(result.current.selectConversation).toHaveBeenCalledWith("dm-worker", { rooms: [directConversation] });
    expect(createUserRequest).not.toHaveBeenCalled();
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

  it("starts Feishu connection by storing the pending registration and opening Feishu", async () => {
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    const workerAgent: AgentLike = {
      ...oldAgent,
      id: "u-dev",
      name: "dev",
      role: "worker",
    };
    try {
      const { result } = renderHook(
        () =>
          useAgentControllerHarness({
            activePane: { type: WorkspacePaneTypes.agent, id: "u-dev" },
            agents: [workerAgent],
          }).controller,
        { wrapper: createWrapper() },
      );

      await act(async () => {
        await result.current.agentViewProps.onStartFeishuConnect?.(workerAgent);
      });

      expect(startFeishuRegistrationRequest).toHaveBeenCalledWith("u-dev");
      expect(openSpy).toHaveBeenCalledWith("https://feishu.example/connect", "_blank", "noopener,noreferrer");
      expect(result.current.agentViewProps.notice).toBe("feishuConnectStarted");
      expect(result.current.agentViewProps.noticeTone).toBe("info");
      expect(result.current.agentViewProps.feishuPendingRegistration?.registration_id).toBe("reg-dev");
      expect(JSON.parse(window.localStorage.getItem(feishuRegistrationStorageKey) || "{}")).toMatchObject({
        "u-dev": {
          agent_id: "u-dev",
          registration_id: "reg-dev",
        },
      });
    } finally {
      openSpy.mockRestore();
    }
  });

  it("finalizes Feishu connection and refreshes the selected agent participants", async () => {
    const workerAgent: AgentLike = {
      ...oldAgent,
      id: "u-dev",
      name: "dev",
      role: "worker",
    };
    const connectedWorker: AgentLike = {
      ...workerAgent,
      participants: [
        {
          agent_id: "u-dev",
          channel: "feishu",
          channel_user_kind: "app_id",
          id: "dev",
          type: "agent",
        },
      ],
    };
    vi.mocked(fetchAgent).mockReset();
    vi.mocked(fetchAgent).mockResolvedValue(connectedWorker);
    window.localStorage.setItem(
      feishuRegistrationStorageKey,
      JSON.stringify({
        "u-dev": {
          agent_id: "u-dev",
          connect_url: "https://feishu.example/connect",
          expires_at: "2999-01-01T00:00:00Z",
          participant_id: "dev",
          registration_id: "reg-dev",
        },
      }),
    );

    const { result } = renderHook(
      () =>
        useAgentControllerHarness({
          activePane: { type: WorkspacePaneTypes.agent, id: "u-dev" },
          agents: [workerAgent],
        }).controller,
      { wrapper: createWrapper() },
    );

    await waitFor(() =>
      expect(result.current.agentViewProps.feishuPendingRegistration?.registration_id).toBe("reg-dev"),
    );

    await act(async () => {
      await result.current.agentViewProps.onFinalizeFeishuConnect?.(workerAgent);
    });

    expect(finalizeFeishuRegistrationRequest).toHaveBeenCalledWith("reg-dev");
    await waitFor(() => expect(result.current.agentViewProps.item?.participants?.[0]?.channel).toBe("feishu"));
    expect(result.current.agentViewProps.notice).toBe("feishuConnectConfigured");
    expect(result.current.agentViewProps.noticeTone).toBe("success");
    expect(window.localStorage.getItem(feishuRegistrationStorageKey)).toBeNull();
  });

  it("disconnects Feishu by deleting the connected participant and refreshing the agent", async () => {
    const workerAgent: AgentLike = {
      ...oldAgent,
      id: "u-dev",
      name: "dev",
      role: "worker",
      participants: [
        {
          agent_id: "u-dev",
          channel: "feishu",
          channel_user_kind: "app_id",
          id: "dev",
          type: "agent",
        },
      ],
    };
    const disconnectedWorker: AgentLike = {
      ...workerAgent,
      participants: [],
    };
    vi.mocked(fetchAgent).mockReset();
    vi.mocked(fetchAgent).mockResolvedValue(disconnectedWorker);

    const { result } = renderHook(
      () =>
        useAgentControllerHarness({
          activePane: { type: WorkspacePaneTypes.agent, id: "u-dev" },
          agents: [workerAgent],
        }).controller,
      { wrapper: createWrapper() },
    );

    await act(async () => {
      await result.current.agentViewProps.onDisconnectFeishu?.(workerAgent);
    });

    expect(deleteFeishuParticipantRequest).toHaveBeenCalledWith("dev");
    await waitFor(() => expect(result.current.agentViewProps.item?.participants).toEqual([]));
    expect(result.current.agentViewProps.notice).toBe("feishuDisconnectConfigured");
    expect(result.current.agentViewProps.noticeTone).toBe("success");
  });

  it("automatically finalizes a pending Feishu connection after the user authorizes in Feishu", async () => {
    vi.useFakeTimers();
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    const workerAgent: AgentLike = {
      ...oldAgent,
      id: "u-dev",
      name: "dev",
      role: "worker",
    };
    const connectedWorker: AgentLike = {
      ...workerAgent,
      participants: [
        {
          agent_id: "u-dev",
          channel: "feishu",
          channel_user_kind: "app_id",
          id: "dev",
          type: "agent",
        },
      ],
    };
    vi.mocked(fetchAgent).mockReset();
    vi.mocked(fetchAgent).mockResolvedValue(connectedWorker);

    try {
      const { result } = renderHook(
        () =>
          useAgentControllerHarness({
            activePane: { type: WorkspacePaneTypes.agent, id: "u-dev" },
            agents: [workerAgent],
          }).controller,
        { wrapper: createWrapper() },
      );

      await act(async () => {
        await result.current.agentViewProps.onStartFeishuConnect?.(workerAgent);
      });

      expect(result.current.agentViewProps.feishuPendingRegistration?.registration_id).toBe("reg-dev");

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1_000);
      });
      vi.useRealTimers();

      expect(finalizeFeishuRegistrationRequest).toHaveBeenCalledWith("reg-dev");
      await waitFor(() => expect(result.current.agentViewProps.item?.participants?.[0]?.channel).toBe("feishu"));
      expect(result.current.agentViewProps.notice).toBe("feishuConnectConfigured");
      expect(result.current.agentViewProps.noticeTone).toBe("success");
      expect(window.localStorage.getItem(feishuRegistrationStorageKey)).toBeNull();
    } finally {
      openSpy.mockRestore();
      vi.useRealTimers();
    }
  });

  it("keeps automatic Feishu polling out of the visible channel busy state", async () => {
    vi.useFakeTimers();
    const openSpy = vi.spyOn(window, "open").mockImplementation(() => null);
    const workerAgent: AgentLike = {
      ...oldAgent,
      id: "u-dev",
      name: "dev",
      role: "worker",
    };
    let resolveFinalize: (value: Awaited<ReturnType<typeof finalizeFeishuRegistrationRequest>>) => void = () => {};
    vi.mocked(finalizeFeishuRegistrationRequest).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFinalize = resolve;
        }),
    );

    try {
      const { result } = renderHook(
        () =>
          useAgentControllerHarness({
            activePane: { type: WorkspacePaneTypes.agent, id: "u-dev" },
            agents: [workerAgent],
          }).controller,
        { wrapper: createWrapper() },
      );

      await act(async () => {
        await result.current.agentViewProps.onStartFeishuConnect?.(workerAgent);
      });

      expect(result.current.agentViewProps.feishuPendingRegistration?.registration_id).toBe("reg-dev");
      expect(result.current.agentViewProps.feishuConnectBusy).toBe("");

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1_000);
      });

      expect(finalizeFeishuRegistrationRequest).toHaveBeenCalledWith("reg-dev");
      expect(result.current.agentViewProps.feishuConnectBusy).toBe("");

      await act(async () => {
        resolveFinalize({
          agent_id: "u-dev",
          participant_id: "dev",
          registration_id: "reg-dev",
          status: "pending",
        });
        await Promise.resolve();
      });
    } finally {
      openSpy.mockRestore();
      vi.useRealTimers();
    }
  });

  it("initializes create agent drafts with an unused built-in avatar", async () => {
    const availableAvatar = AGENT_AVATAR_OPTIONS.at(-1)?.value || "";
    const humanAvatar = AGENT_AVATAR_OPTIONS.at(-2)?.value || "";
    const agents = AGENT_AVATAR_OPTIONS.slice(0, -2).map((option, index): AgentLike => {
      const manager = index === 0;
      return {
        id: manager ? "u-manager" : `u-worker-${index}`,
        avatar: option.value,
        image: oldImage,
        name: manager ? "manager" : `worker-${index}`,
        role: manager ? "manager" : "worker",
        runtime_kind: "picoclaw_sandbox",
        status: "running",
      };
    });

    const { result } = renderHook(
      () =>
        useAgentControllerHarness({
          agents,
          data: {
            current_user_id: "u-admin",
            rooms: [],
            users: [{ id: "u-admin", avatar: humanAvatar, name: "admin" }],
          },
        }).controller,
      { wrapper: createWrapper() },
    );

    await act(async () => {
      await result.current.computerViewProps.onCreateAgent();
    });

    await waitFor(() => expect(result.current.agentProfileModalProps).not.toBeNull());
    expect(result.current.agentProfileModalProps?.agentDraft.avatar).toBe(availableAvatar);
  });
});

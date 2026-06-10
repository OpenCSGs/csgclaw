import { useState, type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import { fetchAgent, fetchAgentProfile, fetchAgentProfileModels, runAgentActionRequest } from "@/api/agents";
import { useAgentController } from "@/hooks/workspace/useAgentController";
import { WorkspacePaneTypes } from "@/models/routing";
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
    runAgentActionRequest: vi.fn(),
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
  runtime_kind: "codex",
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

function useAgentControllerHarness() {
  const [agents, setAgents] = useState<AgentLike[]>([oldAgent]);
  const refreshWorkspaceAgents = vi.fn(async () => [oldAgent]);

  return useAgentController({
    activeConversationId: "",
    activePane: { type: WorkspacePaneTypes.agent, id: "u-manager" },
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
    managerProfile: null,
    refreshHubTemplates: vi.fn(async () => undefined),
    refreshWorkspaceAgents,
    refreshWorkspaceBootstrap: vi.fn(async () => null),
    refreshWorkspaceBootstrapConfig: vi.fn(async () => null),
    refreshWorkspaceManagerProfile: vi.fn(async () => null),
    rooms: [],
    selectComputer: vi.fn(),
    selectConversation: vi.fn(),
    selectHub: vi.fn(),
    setAgentsData: (value) => {
      setAgents((current) => (typeof value === "function" ? value(current) : value));
    },
    setManagerProfileData: vi.fn(),
    setSelectedHubTemplateId: vi.fn(),
    t,
  });
}

describe("useAgentController", () => {
  beforeEach(() => {
    vi.mocked(fetchAgent).mockReset();
    vi.mocked(fetchAgentProfile).mockReset();
    vi.mocked(fetchAgentProfileModels).mockReset();
    vi.mocked(runAgentActionRequest).mockReset();
    vi.mocked(fetchAgent).mockResolvedValueOnce(oldAgent).mockResolvedValueOnce(latestAgent);
    vi.mocked(fetchAgentProfile).mockResolvedValue(profile);
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: [] });
    vi.mocked(runAgentActionRequest).mockResolvedValue({
      ...oldAgent,
      image: actionImage,
    });
  });

  it("refreshes the selected agent detail from a cache-busted agent fetch after upgrade", async () => {
    const { result } = renderHook(() => useAgentControllerHarness(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.agentViewProps.draft?.image).toBe(oldImage));

    await act(async () => {
      await result.current.agentViewProps.onUpgrade?.(oldAgent);
    });

    await waitFor(() => expect(result.current.agentViewProps.item?.image).toBe(latestImage));
    expect(result.current.agentViewProps.draft?.image).toBe(latestImage);
    expect(result.current.agentViewProps.savedDraft?.image).toBe(latestImage);
    expect(runAgentActionRequest).toHaveBeenCalledWith("u-manager", "upgrade");
    expect(fetchAgent).toHaveBeenLastCalledWith("u-manager", { cacheBust: true });
  });
});

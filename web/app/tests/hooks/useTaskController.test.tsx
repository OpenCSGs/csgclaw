import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { fetchGlobalTasks, fetchTeamEvents, fetchTeams } from "@/api/tasks";
import { useTaskController } from "@/hooks/workspace/useTaskController";
import { WorkspacePaneTypes } from "@/models/routing";
import type { WorkspacePane } from "@/models/routing";
import type { TranslateFn } from "@/models/conversations";
import type { WorkspaceTask } from "@/models/tasks";

vi.mock("@/api/tasks", async () => {
  const actual = await vi.importActual<typeof import("@/api/tasks")>("@/api/tasks");
  return {
    ...actual,
    fetchGlobalTasks: vi.fn(async () => []),
    fetchTeamEvents: vi.fn(async () => []),
    fetchTeams: vi.fn(async () => []),
  };
});

const TASKS_QUERY_KEY = ["workspace", "tasks"] as const;
const t: TranslateFn = (key) => key;

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Infinity,
      },
    },
  });
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function renderTaskController(queryClient: QueryClient, activePane: WorkspacePane) {
  return renderHook(
    ({ pane }: { pane: WorkspacePane }) =>
      useTaskController({
        activePane: pane,
        agents: [],
        onSelectConversation: vi.fn(),
        onSelectTask: vi.fn(),
        t,
      }),
    {
      initialProps: { pane: activePane },
      wrapper: createWrapper(queryClient),
    },
  );
}

describe("useTaskController", () => {
  beforeEach(() => {
    vi.mocked(fetchGlobalTasks).mockReset();
    vi.mocked(fetchTeamEvents).mockReset();
    vi.mocked(fetchTeams).mockReset();
    vi.mocked(fetchGlobalTasks).mockResolvedValue([]);
    vi.mocked(fetchTeamEvents).mockResolvedValue([]);
    vi.mocked(fetchTeams).mockResolvedValue([]);
  });

  it("refetches stale cached tasks when the tasks pane becomes active", async () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData<WorkspaceTask[]>(TASKS_QUERY_KEY, [], {
      updatedAt: Date.now() - 6000,
    });
    const { rerender } = renderTaskController(queryClient, {
      type: WorkspacePaneTypes.conversation,
      id: "room-1",
    });

    await act(async () => {
      await Promise.resolve();
    });
    expect(fetchGlobalTasks).not.toHaveBeenCalled();

    rerender({ pane: { type: WorkspacePaneTypes.task, id: "" } });

    await waitFor(() => {
      expect(fetchGlobalTasks).toHaveBeenCalledTimes(1);
    });
  });

  it("keeps recent cached tasks when the tasks pane becomes active", async () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData<WorkspaceTask[]>(TASKS_QUERY_KEY, [], {
      updatedAt: Date.now() - 1000,
    });
    const { rerender } = renderTaskController(queryClient, {
      type: WorkspacePaneTypes.conversation,
      id: "room-1",
    });

    rerender({ pane: { type: WorkspacePaneTypes.task, id: "" } });
    await act(async () => {
      await Promise.resolve();
    });

    expect(fetchGlobalTasks).not.toHaveBeenCalled();
  });

  it("does not immediately retry a failed tab revalidation", async () => {
    vi.mocked(fetchGlobalTasks).mockRejectedValue(new Error("network down"));
    const queryClient = createQueryClient();
    queryClient.setQueryData<WorkspaceTask[]>(TASKS_QUERY_KEY, [], {
      updatedAt: Date.now() - 6000,
    });

    renderTaskController(queryClient, {
      type: WorkspacePaneTypes.task,
      id: "",
    });

    await waitFor(() => {
      expect(fetchGlobalTasks).toHaveBeenCalledTimes(1);
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(fetchGlobalTasks).toHaveBeenCalledTimes(1);
  });
});

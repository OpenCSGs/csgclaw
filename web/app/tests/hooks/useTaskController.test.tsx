import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { fetchGlobalTasks, fetchTeamEvents, fetchTeams } from "@/api/tasks";
import { fetchScheduledTaskRuns, fetchScheduledTasks } from "@/api/scheduledTasks";
import { useTaskController } from "@/hooks/workspace/useTaskController";
import { WorkspacePaneTypes } from "@/models/routing";
import type { WorkspacePane } from "@/models/routing";
import type { TranslateFn } from "@/models/conversations";
import type { WorkspaceTask } from "@/models/tasks";
import type { WorkspaceScheduledTask, WorkspaceScheduledTaskRun } from "@/models/scheduledTasks";

vi.mock("@/api/tasks", async () => {
  const actual = await vi.importActual<typeof import("@/api/tasks")>("@/api/tasks");
  return {
    ...actual,
    fetchGlobalTasks: vi.fn(async () => []),
    fetchTeamEvents: vi.fn(async () => []),
    fetchTeams: vi.fn(async () => []),
  };
});

vi.mock("@/api/scheduledTasks", async () => {
  const actual = await vi.importActual<typeof import("@/api/scheduledTasks")>("@/api/scheduledTasks");
  return {
    ...actual,
    fetchScheduledTasks: vi.fn(async () => []),
    fetchScheduledTaskRuns: vi.fn(async () => []),
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

function task(overrides: Partial<WorkspaceTask> = {}): WorkspaceTask {
  return {
    id: "task-1",
    assignment_type: "team",
    assignment_id: "team-1",
    team_id: "team-1",
    team_title: "Team One",
    execution_channel: "csgclaw",
    room_id: "",
    room_title: "",
    parent_id: "",
    title: "Task one",
    body: "",
    status: "pending",
    created_by: "",
    created_by_agent_name: "",
    assigned_to: "",
    assigned_to_agent_name: "",
    claimed_by: "",
    claimed_by_agent_name: "",
    priority: 0,
    depends_on: [],
    plan_summary: "",
    dispatched_at: "",
    result: "",
    error: "",
    created_at: "2026-07-08T00:00:00Z",
    updated_at: "2026-07-08T00:00:00Z",
    ...overrides,
  };
}

function scheduledTask(overrides: Partial<WorkspaceScheduledTask> = {}): WorkspaceScheduledTask {
  return {
    id: "scheduled-task-1",
    title: "Morning report",
    agent_id: "agent-1",
    prompt: "Write the report",
    recurrence: "daily",
    enabled: true,
    next_run_at: "2026-07-08T09:00:00Z",
    last_run_at: "",
    expires_at: "",
    created_at: "2026-07-08T00:00:00Z",
    updated_at: "2026-07-08T00:00:00Z",
    ...overrides,
  };
}

function scheduledRun(overrides: Partial<WorkspaceScheduledTaskRun> = {}): WorkspaceScheduledTaskRun {
  return {
    id: "scheduled-run-1",
    scheduled_task_id: "scheduled-task-1",
    triggered_at: "2026-07-08T09:00:00Z",
    status: "triggered",
    task_id: "task-scheduled-run-1",
    error: "",
    ...overrides,
  };
}

describe("useTaskController", () => {
  beforeEach(() => {
    vi.mocked(fetchGlobalTasks).mockReset();
    vi.mocked(fetchTeamEvents).mockReset();
    vi.mocked(fetchTeams).mockReset();
    vi.mocked(fetchScheduledTasks).mockReset();
    vi.mocked(fetchScheduledTaskRuns).mockReset();
    vi.mocked(fetchGlobalTasks).mockResolvedValue([]);
    vi.mocked(fetchTeamEvents).mockResolvedValue([]);
    vi.mocked(fetchTeams).mockResolvedValue([]);
    vi.mocked(fetchScheduledTasks).mockResolvedValue([]);
    vi.mocked(fetchScheduledTaskRuns).mockResolvedValue([]);
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

  it("refreshes scheduled task state while background task polling is active", async () => {
    vi.useFakeTimers();
    try {
      const queryClient = createQueryClient();
      const activeTask = task({ status: "in_progress" });
      const nextScheduledTask = scheduledTask({ last_run_at: "2026-07-08T09:00:00Z" });
      const nextRun = scheduledRun();
      vi.mocked(fetchGlobalTasks).mockResolvedValue([activeTask]);
      vi.mocked(fetchScheduledTasks).mockResolvedValue([nextScheduledTask]);
      vi.mocked(fetchScheduledTaskRuns).mockResolvedValue([nextRun]);

      const { result } = renderTaskController(queryClient, {
        type: WorkspacePaneTypes.task,
        id: activeTask.id,
      });

      await vi.waitFor(() => {
        expect(result.current.taskViewProps.tasks).toHaveLength(1);
      });
      vi.mocked(fetchScheduledTasks).mockClear();
      vi.mocked(fetchScheduledTaskRuns).mockClear();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(3000);
      });

      await vi.waitFor(() => {
        expect(fetchScheduledTasks).toHaveBeenCalledTimes(1);
      });
      expect(fetchScheduledTaskRuns).toHaveBeenCalledWith(nextScheduledTask.id);
      expect(queryClient.getQueryData(["workspace", "scheduled-task-runs", nextScheduledTask.id])).toEqual([nextRun]);
    } finally {
      vi.useRealTimers();
    }
  });
});

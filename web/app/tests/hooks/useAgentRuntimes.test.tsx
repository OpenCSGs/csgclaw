import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fetchAgentRuntimes } from "@/api/agentRuntimes";
import { useAgentRuntimes } from "@/pages/ComputerPage/useAgentRuntimes";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import type { TranslateFn } from "@/models/conversations";

vi.mock("@/api/agentRuntimes", () => ({ fetchAgentRuntimes: vi.fn() }));

const t: TranslateFn = (key) => {
  if (key === "computerRuntimesLoadFailed") {
    return "Failed to load agent runtimes. Please retry.";
  }
  return key;
};

const bundledCodex = {
  name: "codex",
  label: "Codex CLI",
  supported: true,
  installed: true,
  installable: false,
  status: "installed",
  path: "/opt/csgclaw/bin/codex",
};

const claudeCode = {
  name: "claude_code",
  label: "Claude Code",
  supported: false,
  installed: false,
  installable: false,
  status: "coming_soon",
};

function createHarness() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return { queryClient, wrapper };
}

describe("useAgentRuntimes", () => {
  beforeEach(() => {
    vi.mocked(fetchAgentRuntimes).mockReset();
  });

  it("loads the bundled runtime and refreshes bootstrap readiness", async () => {
    vi.mocked(fetchAgentRuntimes).mockResolvedValue([bundledCodex, claudeCode]);
    const { queryClient, wrapper } = createHarness();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useAgentRuntimes(t), { wrapper });

    await waitFor(() => expect(result.current.runtimes).toHaveLength(2));
    expect(result.current.runtimes.find((runtime) => runtime.name === "codex")).toMatchObject({
      installed: true,
      installable: false,
      path: "/opt/csgclaw/bin/codex",
    });
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: workspaceQueryKeys.bootstrapConfig() }));
  });

  it("surfaces load failures and supports an explicit status refresh", async () => {
    vi.mocked(fetchAgentRuntimes)
      .mockRejectedValueOnce(new Error("runtime service unavailable"))
      .mockResolvedValueOnce([bundledCodex, claudeCode]);
    const { wrapper } = createHarness();
    const { result } = renderHook(() => useAgentRuntimes(t), { wrapper });

    await waitFor(() => expect(result.current.error).toBe("runtime service unavailable"));
    await result.current.refresh();
    await waitFor(() => expect(result.current.runtimes).toHaveLength(2));
  });
});

import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fetchAgentRuntimes, installAgentRuntimeRequest } from "@/api/agentRuntimes";
import { useAgentRuntimes } from "@/pages/ComputerPage/useAgentRuntimes";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import type { TranslateFn } from "@/models/conversations";

vi.mock("@/api/agentRuntimes", async () => {
  const actual = await vi.importActual<typeof import("@/api/agentRuntimes")>("@/api/agentRuntimes");
  return {
    ...actual,
    fetchAgentRuntimes: vi.fn(),
    installAgentRuntimeRequest: vi.fn(),
  };
});

const t: TranslateFn = (key) => {
  if (key === "computerRuntimeInstallFailed") {
    return "Failed to install Codex CLI. Please retry.";
  }
  if (key === "computerRuntimesLoadFailed") {
    return "Failed to load agent runtimes. Please retry.";
  }
  return key;
};

const missingCodex = {
  name: "codex",
  label: "Codex CLI",
  supported: true,
  installed: false,
  installable: true,
  status: "not_installed",
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
    vi.mocked(installAgentRuntimeRequest).mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("updates the runtime cache and invalidates bootstrap readiness after install", async () => {
    vi.mocked(fetchAgentRuntimes).mockResolvedValue([missingCodex, claudeCode]);
    vi.mocked(installAgentRuntimeRequest).mockResolvedValue({
      ...missingCodex,
      installed: true,
      status: "installed",
      path: "/opt/csgclaw/codex",
    });
    const { queryClient, wrapper } = createHarness();
    const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useAgentRuntimes(t), { wrapper });

    await waitFor(() => expect(result.current.runtimes).toHaveLength(2));
    await act(async () => {
      await result.current.installRuntime("codex");
    });

    await waitFor(() =>
      expect(result.current.runtimes.find((runtime) => runtime.name === "codex")).toMatchObject({
        installed: true,
        path: "/opt/csgclaw/codex",
      }),
    );
    await waitFor(() => expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: workspaceQueryKeys.bootstrapConfig() }));
    expect(installAgentRuntimeRequest).toHaveBeenCalledWith("codex");
    expect(result.current.installError).toBe("");
  });

  it("surfaces a failed install and lets the same manual action retry", async () => {
    vi.mocked(fetchAgentRuntimes)
      .mockResolvedValueOnce([missingCodex, claudeCode])
      .mockResolvedValueOnce([
        { ...missingCodex, status: "failed", message: "download service unavailable" },
        claudeCode,
      ]);
    vi.mocked(installAgentRuntimeRequest)
      .mockRejectedValueOnce({ status: 502, message: "download service unavailable" })
      .mockResolvedValueOnce({
        ...missingCodex,
        installed: true,
        status: "installed",
        path: "/opt/csgclaw/codex",
      });
    const { wrapper } = createHarness();
    const { result } = renderHook(() => useAgentRuntimes(t), { wrapper });

    await waitFor(() => expect(result.current.runtimes).toHaveLength(2));
    await act(async () => {
      await result.current.installRuntime("codex");
    });

    expect(result.current.installError).toBe("download service unavailable");
    expect(result.current.runtimes.find((runtime) => runtime.name === "codex")?.status).toBe("failed");

    await act(async () => {
      await result.current.installRuntime("codex");
    });

    expect(installAgentRuntimeRequest).toHaveBeenCalledTimes(2);
    expect(result.current.installError).toBe("");
    expect(result.current.runtimes.find((runtime) => runtime.name === "codex")?.installed).toBe(true);
  });

  it("polls a first not-installed response until the serve-time install becomes visible", async () => {
    vi.mocked(fetchAgentRuntimes)
      .mockResolvedValueOnce([missingCodex, claudeCode])
      .mockResolvedValueOnce([
        {
          ...missingCodex,
          installed: true,
          status: "installed",
          path: "/opt/csgclaw/codex",
        },
        claudeCode,
      ]);
    const { wrapper } = createHarness();
    const { result } = renderHook(() => useAgentRuntimes(t), { wrapper });

    await waitFor(() =>
      expect(result.current.runtimes.find((runtime) => runtime.name === "codex")?.installed).toBe(false),
    );
    await waitFor(
      () => expect(result.current.runtimes.find((runtime) => runtime.name === "codex")?.installed).toBe(true),
      { timeout: 3000 },
    );
    expect(fetchAgentRuntimes).toHaveBeenCalledTimes(2);
  });

  it("deduplicates rapid install clicks while the first request is active", async () => {
    vi.mocked(fetchAgentRuntimes).mockResolvedValue([missingCodex, claudeCode]);
    let resolveInstall: ((value: unknown) => void) | undefined;
    vi.mocked(installAgentRuntimeRequest).mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveInstall = resolve;
        }),
    );
    const { wrapper } = createHarness();
    const { result } = renderHook(() => useAgentRuntimes(t), { wrapper });

    await waitFor(() => expect(result.current.runtimes).toHaveLength(2));
    await act(async () => {
      const first = result.current.installRuntime("codex");
      const duplicate = result.current.installRuntime("codex");
      resolveInstall?.({ ...missingCodex, installed: true, status: "installed" });
      await Promise.all([first, duplicate]);
    });

    expect(installAgentRuntimeRequest).toHaveBeenCalledTimes(1);
  });
});

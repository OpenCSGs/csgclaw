import { useState, type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fetchAgentProfileModels } from "@/api/agents";
import { useProfileModelOptions } from "@/hooks/workspace/useProfileModelOptions";
import type { AgentDraft } from "@/models/agents";

vi.mock("@/api/agents", async () => {
  const actual = await vi.importActual<typeof import("@/api/agents")>("@/api/agents");
  return {
    ...actual,
    fetchAgentProfileModels: vi.fn(),
  };
});

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

function apiDraft(overrides: Partial<AgentDraft> = {}): AgentDraft {
  return {
    api_key: "",
    api_key_preview: "",
    api_key_set: false,
    base_url: "",
    enable_fast_mode: false,
    envRows: [],
    headersText: "{}",
    model_id: "",
    provider: "api",
    reasoning_effort: "medium",
    requestOptionsText: "{}",
    runtime_kind: "picoclaw_sandbox",
    ...overrides,
  };
}

describe("useProfileModelOptions", () => {
  beforeEach(() => {
    vi.mocked(fetchAgentProfileModels).mockReset();
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: [] });
  });

  it("waits for API provider connection details before probing models", async () => {
    renderHook(
      () =>
        useProfileModelOptions({
          draft: apiDraft(),
          onDraftChange: vi.fn(),
        }),
      { wrapper: createWrapper() },
    );

    await new Promise((resolve) => window.setTimeout(resolve, 520));

    expect(fetchAgentProfileModels).not.toHaveBeenCalled();
  });

  it("probes API provider models with the latest draft and selects the first returned model", async () => {
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: ["gpt-auto"] });

    function useHarness() {
      const [draft, setDraft] = useState<AgentDraft | null>(
        apiDraft({
          api_key: "test-key",
          base_url: "https://models.example.test/v1",
        }),
      );
      return {
        draft,
        ...useProfileModelOptions({
          draft,
          onDraftChange: setDraft,
        }),
      };
    }

    const { result } = renderHook(() => useHarness(), { wrapper: createWrapper() });

    await waitFor(() =>
      expect(fetchAgentProfileModels).toHaveBeenCalledWith(
        expect.objectContaining({
          api_key: "test-key",
          base_url: "https://models.example.test/v1",
          provider: "api",
        }),
      ),
    );
    await waitFor(() => expect(result.current.draft?.model_id).toBe("gpt-auto"));
  });
});

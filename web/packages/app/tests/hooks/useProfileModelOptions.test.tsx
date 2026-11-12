import { useState, type ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
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
    runtime_kind: "codex",
    ...overrides,
  };
}

describe("useProfileModelOptions", () => {
  it("does not carry a previous provider's authentication error into a new draft", async () => {
    const error = { status: 401, message: "Invalid provider key" };
    vi.mocked(fetchAgentProfileModels).mockRejectedValue(error);
    const { result, rerender } = renderHook(({ draft }) => useProfileModelOptions({ draft }), {
      initialProps: { draft: apiDraft({ provider: "codex" }) },
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(result.current.modelError).toEqual(error));
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: ["new-model"] });
    rerender({ draft: apiDraft({ provider: "csghub" }) });
    expect(result.current.modelError).toBeNull();
    await waitFor(() => expect(result.current.models).toEqual(["new-model"]));
    expect(result.current.modelError).toBeNull();
  });

  it("drops retained query data when refreshing models fails", async () => {
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: ["previously-available"] });
    const draft = apiDraft({ provider: "codex", model_id: "saved-model" });
    const { result } = renderHook(() => useProfileModelOptions({ draft }), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.models).toEqual(["previously-available"]));
    vi.mocked(fetchAgentProfileModels).mockRejectedValue(new Error("Codex auth was not found"));
    await act(async () => {
      await result.current.retryModels();
    });
    await waitFor(() => expect(result.current.modelError).toBeTruthy());
    expect(result.current.models).toEqual([]);
    expect(draft.model_id).toBe("saved-model");
  });

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

  it("probes a catalog provider by ID without requiring connection details in the agent draft", async () => {
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: ["catalog-model"] });
    const { result } = renderHook(
      () => useProfileModelOptions({ draft: apiDraft({ model_provider_id: "catalog-provider" }) }),
      { wrapper: createWrapper() },
    );
    await waitFor(() => expect(result.current.models).toEqual(["catalog-model"]));
    expect(fetchAgentProfileModels).toHaveBeenCalledWith(
      expect.objectContaining({ model_provider_id: "catalog-provider", base_url: "", api_key: "" }),
    );
  });

  it("ignores an old provider's response after switching to another catalog provider", async () => {
    let resolvePrevious!: (value: { models: string[] }) => void;
    vi.mocked(fetchAgentProfileModels).mockReturnValueOnce(
      new Promise((resolve) => {
        resolvePrevious = resolve;
      }),
    );
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: ["new-provider-model"] });
    const { result, rerender } = renderHook(({ draft }) => useProfileModelOptions({ draft }), {
      initialProps: { draft: apiDraft({ model_provider_id: "previous" }) },
      wrapper: createWrapper(),
    });
    await waitFor(() => expect(fetchAgentProfileModels).toHaveBeenCalledTimes(1));
    rerender({ draft: apiDraft({ model_provider_id: "next" }) });
    expect(result.current.models).toEqual([]);
    await waitFor(() => expect(result.current.models).toEqual(["new-provider-model"]));
    await act(async () => resolvePrevious({ models: ["previous-provider-model"] }));
    expect(result.current.models).toEqual(["new-provider-model"]);
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

  it("probes CSGHub models without manual API fields", async () => {
    vi.mocked(fetchAgentProfileModels).mockResolvedValue({ models: ["deepseek-v4-flash"] });

    renderHook(
      () =>
        useProfileModelOptions({
          draft: apiDraft({ provider: "csghub" }),
          onDraftChange: vi.fn(),
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() =>
      expect(fetchAgentProfileModels).toHaveBeenCalledWith(
        expect.objectContaining({
          api_key: "",
          base_url: "",
          provider: "csghub",
        }),
      ),
    );
  });
});

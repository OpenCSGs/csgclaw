import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { checkModelProvider, fetchModelProviders, type ModelProviderCheckResult } from "@/api/modelProviders";
import { useWorkspaceModelProviderCatalog } from "@/hooks/workspace/useWorkspaceModelProviderCatalog";
import { normalizeModelProviderCatalog, type ModelProvider, type ModelProviderCatalog } from "@/models/modelProviders";

vi.mock("@/api/modelProviders", () => ({ checkModelProvider: vi.fn(), fetchModelProviders: vi.fn() }));

function pendingResponse<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function catalog(...providers: Array<Partial<ModelProvider>>) {
  return normalizeModelProviderCatalog({ providers });
}

function renderCatalog(initial: ModelProviderCatalog) {
  vi.mocked(fetchModelProviders).mockReset().mockResolvedValue(initial);
  vi.mocked(checkModelProvider).mockReset();
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(useWorkspaceModelProviderCatalog, { wrapper });
}

it("refreshes the full catalog after a check overlaps a provider deletion", async () => {
  const a = { id: "a", models: ["old-model"] };
  const b = { id: "b", models: ["removed-model"] };
  const initial = catalog(a, b);
  const updated = catalog({ ...a, models: ["checked-model"], status: "connected" });
  const checked = updated.providers[0];
  const { result } = renderCatalog(initial);
  await waitFor(() => expect(result.current.modelProvidersQuery.data).toEqual(initial));

  const check = pendingResponse<ModelProviderCheckResult>();
  const olderRead = pendingResponse<ModelProviderCatalog>();
  const restartedRead = pendingResponse<ModelProviderCatalog>();
  vi.mocked(checkModelProvider).mockReturnValueOnce(check.promise);
  vi.mocked(fetchModelProviders).mockReturnValueOnce(olderRead.promise).mockReturnValueOnce(restartedRead.promise);

  const checking = result.current.checkWorkspaceModelProvider(a.id, {});
  const refreshing = result.current.refreshWorkspaceModelProviders();
  await act(async () => {
    check.resolve({ ...checked, provider: checked });
    await checking;
  });

  expect(fetchModelProviders).toHaveBeenCalledTimes(3);
  await waitFor(() =>
    expect(result.current.modelProvidersQuery.data?.providers).toEqual([checked, initial.providers[1]]),
  );
  await act(async () => {
    restartedRead.resolve(updated);
    olderRead.resolve(catalog(a));
    await refreshing;
  });
  await waitFor(() => expect(result.current.modelProvidersQuery.data).toEqual(updated));
});

it("keeps a provider saved after an older check started", async () => {
  const a = { id: "a", display_name: "Before", base_url: "https://old.example.test", models: ["old-model"] };
  const initial = catalog(a);
  const saved = catalog({ ...a, display_name: "After", base_url: "https://new.example.test", models: ["saved-model"] });
  const checked = catalog({ ...a, models: ["checked-model"], status: "connected" }).providers[0];
  const { result } = renderCatalog(initial);
  await waitFor(() => expect(result.current.modelProvidersQuery.data).toEqual(initial));

  const check = pendingResponse<ModelProviderCheckResult>();
  const reconciliation = pendingResponse<ModelProviderCatalog>();
  vi.mocked(checkModelProvider).mockReturnValueOnce(check.promise);
  vi.mocked(fetchModelProviders).mockResolvedValueOnce(saved).mockReturnValueOnce(reconciliation.promise);

  const checking = result.current.checkWorkspaceModelProvider(a.id, {});
  await act(async () => {
    await result.current.refreshWorkspaceModelProviders();
  });
  await waitFor(() => expect(result.current.modelProvidersQuery.data).toEqual(saved));
  await act(async () => {
    check.resolve({ ...checked, provider: checked });
    expect((await checking).savedProvider).toEqual(saved.providers[0]);
  });

  expect(fetchModelProviders).toHaveBeenCalledTimes(3);
  expect(result.current.modelProvidersQuery.data).toEqual(saved);
  await act(async () => reconciliation.resolve(saved));
});

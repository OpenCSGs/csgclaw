import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { fetchHubTemplates } from "@/api/hub";
import type { HubTemplate } from "@/models/hubWorkspace";
import { useWorkspaceHubTemplatesQuery, workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";

vi.mock("@/api/hub", async (original) => ({
  ...(await original<typeof import("@/api/hub")>()),
  fetchHubTemplates: vi.fn(),
}));

describe("progressive template loading", () => {
  it("shows local templates before community requests finish", async () => {
    const local: HubTemplate = { id: "local.demo", name: "demo", source: { kind: "local" } };
    const remote: HubTemplate = { id: "alice/demo", name: "demo", source: { kind: "remote" } };
    let finish!: (items: HubTemplate[]) => void;
    vi.mocked(fetchHubTemplates).mockImplementation((scope) =>
      scope === "local"
        ? Promise.resolve([local])
        : new Promise((resolve) => {
            finish = resolve;
          }),
    );
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useWorkspaceHubTemplatesQuery(), { wrapper });
    await waitFor(() => expect(result.current.data).toEqual([local]));
    expect(result.current.isLoading).toBe(false);
    expect(result.current.isFetching).toBe(true);
    finish([remote]);
    await waitFor(() => expect(result.current.data).toEqual([local, remote]));
  });
  it("reports community errors while retaining local and cached community templates, then retries", async () => {
    const local: HubTemplate = { id: "local.demo", name: "demo", source: { kind: "local" } };
    const cached: HubTemplate = { id: "alice/cached", name: "cached", source: { kind: "remote" } };
    const remote: HubTemplate = { id: "agentichub/new", name: "new", source: { kind: "remote" } };
    let fail = true;
    vi.mocked(fetchHubTemplates).mockImplementation(async (scope) => {
      if (scope === "local") return [local];
      if (fail) throw new Error("community returned HTTP 502");
      return [remote];
    });
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    client.setQueryData(workspaceQueryKeys.hubTemplates(), [cached], { updatedAt: 1 });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useWorkspaceHubTemplatesQuery(), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toEqual([local, cached]);
    expect(result.current.isFetching).toBe(false);
    fail = false;
    await act(async () => {
      await result.current.refetch();
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([local, remote]);
  });
});

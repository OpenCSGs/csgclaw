import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fetchRemoteKnowledgeBaseMCPConfig, fetchRemoteKnowledgeBases } from "@/api/knowledgeBases";
import { useWorkspaceKnowledgeBaseSelection } from "@/hooks/workspace/useWorkspaceKnowledgeBaseSelection";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";

vi.mock("@/api/knowledgeBases", () => ({
  fetchRemoteKnowledgeBaseMCPConfig: vi.fn(),
  fetchRemoteKnowledgeBases: vi.fn(),
}));

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe("useWorkspaceKnowledgeBaseSelection", () => {
  it("searches existing knowledge bases by Chinese text and clears the filter", async () => {
    vi.mocked(fetchRemoteKnowledgeBases).mockImplementation(async (search = "") => ({
      items:
        search && search !== "旅游"
          ? []
          : [{ id: "142", contentID: "tourism", name: "旅游知识库", availability: "available" as const }],
      page: 1,
      per: 50,
      total: search && search !== "旅游" ? 0 : 1,
    }));
    const { result } = renderHook(
      () =>
        useWorkspaceKnowledgeBaseSelection({
          enabled: true,
          openCSGAuthGuard: openCSGAuthGuardStub(),
          openCreateMCPDialog: vi.fn(),
          selectedKnowledgeBaseID: "",
          setSelectedKnowledgeBaseID: vi.fn(),
          t: (key) => key,
        }),
      { wrapper: createWrapper() },
    );
    await waitFor(() => expect(result.current.discoveryItems).toHaveLength(1));
    act(() => result.current.setSearch(" 旅游 "));
    await waitFor(() => expect(fetchRemoteKnowledgeBases).toHaveBeenCalledWith("旅游", 1));
    expect(result.current.discoveryItems[0]?.name).toBe("旅游知识库");
    act(() => result.current.setSearch("missing"));
    await waitFor(() => expect(result.current.discoveryItems).toHaveLength(0));
    act(() => result.current.setSearch(""));
    await waitFor(() => expect(result.current.discoveryItems).toHaveLength(1));
  });
  beforeEach(() => {
    vi.mocked(fetchRemoteKnowledgeBaseMCPConfig).mockReset();
    vi.mocked(fetchRemoteKnowledgeBases).mockReset();
  });

  it("loads every catalog page so configured knowledge bases after page one stay visible", async () => {
    vi.mocked(fetchRemoteKnowledgeBases).mockImplementation(async (_search, page = 1) => {
      if (page === 1) {
        return {
          items: [
            {
              availability: "available",
              contentID: "unconfigured-content",
              id: "unconfigured",
              name: "Unconfigured knowledge base",
            },
          ],
          nextPage: 2,
          page: 1,
          per: 1,
          total: 2,
        };
      }
      return {
        items: [
          {
            availability: "available",
            configuredMCPName: "configured-content",
            contentID: "configured-content",
            id: "configured",
            name: "Configured knowledge base",
          },
        ],
        page: 2,
        per: 1,
        total: 2,
      };
    });

    const { result } = renderHook(
      () =>
        useWorkspaceKnowledgeBaseSelection({
          enabled: true,
          openCSGAuthGuard: openCSGAuthGuardStub(),
          openCreateMCPDialog: vi.fn(),
          selectedKnowledgeBaseID: "",
          setSelectedKnowledgeBaseID: vi.fn(),
          t: (key) => key,
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(fetchRemoteKnowledgeBases).toHaveBeenCalledWith("", 2));
    await waitFor(() => expect(result.current.items.map((item) => item.id)).toEqual(["configured"]));
  });

  it("explains the MCP handoff before preparing and opening the prefilled configuration", async () => {
    vi.mocked(fetchRemoteKnowledgeBases).mockResolvedValue({
      items: [
        {
          availability: "available",
          contentID: "content-42",
          id: "42",
          name: "Investment handbook",
        },
      ],
      page: 1,
      per: 50,
      total: 1,
    });
    vi.mocked(fetchRemoteKnowledgeBaseMCPConfig).mockResolvedValue({
      name: "kb-investment",
      config: { type: "remote", url: "https://example.test/mcp" },
    });
    const openCreateMCPDialog = vi.fn();

    const { result } = renderHook(
      () =>
        useWorkspaceKnowledgeBaseSelection({
          enabled: true,
          openCSGAuthGuard: openCSGAuthGuardStub(),
          openCreateMCPDialog,
          selectedKnowledgeBaseID: "42",
          setSelectedKnowledgeBaseID: vi.fn(),
          t: (key) => key,
        }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(result.current.discoveryItems).toHaveLength(1));
    await act(async () => {
      await result.current.requestMCPConfig("42");
    });

    expect(result.current.pendingMCPKnowledgeBase?.name).toBe("Investment handbook");
    expect(fetchRemoteKnowledgeBaseMCPConfig).not.toHaveBeenCalled();
    expect(openCreateMCPDialog).not.toHaveBeenCalled();

    await act(async () => {
      await result.current.confirmMCPConfig();
    });

    expect(fetchRemoteKnowledgeBaseMCPConfig).toHaveBeenCalledWith("42");
    expect(openCreateMCPDialog).toHaveBeenCalledWith(
      '{\n  "mcpServers": {\n    "kb-investment": {\n      "type": "remote",\n      "url": "https://example.test/mcp"\n    }\n  }\n}',
      "knowledge",
    );
    expect(result.current.pendingMCPKnowledgeBase).toBeNull();
  });
});

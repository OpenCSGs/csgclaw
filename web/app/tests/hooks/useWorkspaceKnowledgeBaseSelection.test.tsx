import type { ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fetchRemoteKnowledgeBases } from "@/api/knowledgeBases";
import { useWorkspaceKnowledgeBaseSelection } from "@/hooks/workspace/useWorkspaceKnowledgeBaseSelection";

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
  beforeEach(() => {
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
          authenticated: true,
          enabled: true,
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
});

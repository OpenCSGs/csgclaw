import type { ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMCPServerRequest } from "@/api/mcp";
import { useWorkspaceMCPSelection } from "@/hooks/workspace/useWorkspaceMCPSelection";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";

vi.mock("@/api/mcp", async () => ({
  ...(await vi.importActual<typeof import("@/api/mcp")>("@/api/mcp")),
  createMCPServerRequest: vi.fn(),
}));

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(client, "invalidateQueries");
  const selectType = vi.fn();
  const selectMCP = vi.fn();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const hook = renderHook(
    () =>
      useWorkspaceMCPSelection({
        selectedMCPServerName: "",
        selectedHubResourceType: "knowledge",
        setSelectedMCPServerName: selectMCP,
        setSelectedHubResourceType: selectType,
        skillCount: 0,
        templateCount: 0,
        enabled: false,
        openCSGAuthGuard: openCSGAuthGuardStub(),
        t: (key) => key,
      }),
    { wrapper },
  );
  return { ...hook, client, invalidate, selectType, selectMCP };
}

const payload = { name: "kb-docs", config: { url: "https://example.test/mcp" } };

describe("MCP creation source", () => {
  it("keeps knowledge base creation on knowledge and refreshes the list after saving", async () => {
    vi.mocked(createMCPServerRequest).mockResolvedValue({ mcpServers: {} });
    const { result, client, invalidate, selectType, selectMCP } = setup();
    act(() => result.current.openCreateMCPDialog("draft", "knowledge"));
    expect(selectType).toHaveBeenLastCalledWith("knowledge");
    await act(async () => {
      expect(await result.current.createMCPServer(payload)).toBe(true);
    });
    expect(selectType).toHaveBeenLastCalledWith("knowledge");
    expect(selectMCP).not.toHaveBeenCalledWith("kb-docs");
    expect(result.current.knowledgeBaseAdded).toBe(true);
    expect(result.current.mcpCreateDialogOpen).toBe(false);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: workspaceQueryKeys.knowledgeBasesScope() });
    act(() => result.current.openCreateMCPDialog());
    expect(result.current.mcpCreateSource).toBe("mcp");
    expect(result.current.knowledgeBaseAdded).toBe(false);
    await act(async () => {
      await result.current.createMCPServer(payload);
    });
    expect(selectType).toHaveBeenLastCalledWith("mcp");
    expect(selectMCP).toHaveBeenLastCalledWith("");
    expect(result.current.mcpAdded).toBe(true);
    expect(result.current.mcpCreateDialogOpen).toBe(false);
    client.clear();
  });

  it("keeps the knowledge draft and dialog on failure, then allows retry", async () => {
    vi.mocked(createMCPServerRequest).mockRejectedValueOnce(new Error("Save failed"));
    const { result, client, selectMCP } = setup();
    act(() => result.current.openCreateMCPDialog("draft", "knowledge"));
    await act(async () => {
      expect(await result.current.createMCPServer(payload)).toBe(false);
    });
    expect(result.current.mcpCreateDialogOpen).toBe(true);
    expect(result.current.mcpCreateInitialDocument).toBe("draft");
    expect(result.current.mcpCreateError).toBe("Save failed");
    expect(result.current.knowledgeBaseAdded).toBe(false);
    expect(selectMCP).not.toHaveBeenCalledWith("kb-docs");
    vi.mocked(createMCPServerRequest).mockResolvedValueOnce({ mcpServers: {} });
    await act(async () => {
      expect(await result.current.createMCPServer(payload)).toBe(true);
    });
    expect(result.current.mcpCreateError).toBe("");
    expect(result.current.knowledgeBaseAdded).toBe(true);
    client.clear();
  });
});

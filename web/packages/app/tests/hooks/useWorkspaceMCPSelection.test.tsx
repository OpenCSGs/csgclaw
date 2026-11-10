import type { ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMCPServerRequest, installRemoteMCPServerRequest, updateMCPServerRequest } from "@/api/mcp";
import { useWorkspaceMCPSelection } from "@/hooks/workspace/useWorkspaceMCPSelection";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";

vi.mock("@/api/mcp", async () => ({
  ...(await vi.importActual<typeof import("@/api/mcp")>("@/api/mcp")),
  createMCPServerRequest: vi.fn(),
  installRemoteMCPServerRequest: vi.fn(),
  updateMCPServerRequest: vi.fn(),
}));

function setup(authGuard = openCSGAuthGuardStub()) {
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
        openCSGAuthGuard: authGuard,
        t: (key) => key,
      }),
    { wrapper },
  );
  return { ...hook, client, invalidate, selectType, selectMCP };
}

const payload = { name: "kb-docs", config: { url: "https://example.test/mcp" } };

afterEach(() => vi.clearAllMocks());

describe("remote MCP installation feedback", () => {
  const item = { id: "user:39", name: "kb-tourism" };

  it("keeps a visible scoped error even when the auth guard handles a 401", async () => {
    const auth = openCSGAuthGuardStub();
    auth.handleAuthenticationError = vi.fn(() => true);
    vi.mocked(installRemoteMCPServerRequest).mockRejectedValueOnce({ status: 401, message: "Sign in required" });
    const { result, client } = setup(auth);
    await act(async () => expect(await result.current.installRemoteMCPServer(item)).toBe(false));
    expect(auth.handleAuthenticationError).toHaveBeenCalled();
    expect(result.current.remoteMCPInstallFailure).toEqual({
      id: item.id,
      message: "resourcesMCPRemoteInstallSignInRequired",
    });
    expect(result.current.mcpMutationError).toBe("resourcesMCPRemoteInstallSignInRequired");
    expect(result.current.remoteMCPInstallBusy).toBe("");
    client.clear();
  });

  it("stops loading on success even while resource discovery is still refreshing", async () => {
    vi.mocked(installRemoteMCPServerRequest).mockResolvedValueOnce("mcp_fixed");
    const { result, client, invalidate } = setup();
    invalidate.mockImplementation(() => new Promise(() => {}));
    await act(async () => expect(await result.current.installRemoteMCPServer(item)).toBe(true));
    expect(result.current.remoteMCPInstallBusy).toBe("");
    expect(result.current.mcpMutationBusy).toBe(false);
    expect(result.current.remoteMCPInstallFailure).toBeNull();
    expect(result.current.mcpAdded).toBe(true);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: workspaceQueryKeys.mcpServers() });
    client.clear();
  });

  it("clears a previous installation failure after a successful retry", async () => {
    vi.mocked(installRemoteMCPServerRequest)
      .mockRejectedValueOnce(new Error("Endpoint unavailable"))
      .mockResolvedValueOnce("mcp_fixed");
    const { result, client } = setup();
    await act(async () => expect(await result.current.installRemoteMCPServer(item)).toBe(false));
    expect(result.current.remoteMCPInstallFailure?.message).toBe("Endpoint unavailable");
    await act(async () => expect(await result.current.installRemoteMCPServer(item)).toBe(true));
    expect(result.current.remoteMCPInstallFailure).toBeNull();
    expect(result.current.mcpMutationError).toBe("");
    client.clear();
  });
});

describe("MCP creation source", () => {
  it("keeps settings knowledge-base discovery enabled when creating or updating in place", async () => {
    vi.mocked(createMCPServerRequest).mockResolvedValue({ mcpServers: {} });
    vi.mocked(updateMCPServerRequest).mockResolvedValue({ mcpServers: {} });
    const { result, client, selectType } = setup();
    await act(async () => {
      expect(await result.current.createMCPServer(payload, { preserveSelection: true })).toBe(true);
      expect(await result.current.updateMCPServer(payload.name, payload, { preserveSelection: true })).toBe(true);
    });
    expect(selectType).not.toHaveBeenCalled();
    client.clear();
  });
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

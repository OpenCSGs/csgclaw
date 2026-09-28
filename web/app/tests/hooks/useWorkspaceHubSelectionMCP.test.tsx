import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fetchMCPServers } from "@/api/mcp";
import { useWorkspaceHubSelection } from "@/hooks/workspace/useWorkspaceHubSelection";
import { useWorkspaceUiStore } from "@/hooks/workspace/workspaceUiStore";
import { paneFromLocation } from "@/models/routing";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";

vi.mock("@/api/mcp", async () => ({
  ...(await vi.importActual<typeof import("@/api/mcp")>("@/api/mcp")),
  fetchMCPServers: vi.fn(),
}));

const catalog = {
  mcpServers: {
    filesystem: { command: "npx", args: ["server-filesystem", "${workspace}"] },
    search: { url: "https://example.test/mcp" },
  },
  probes_pending: false,
};
const t = (key: string) => key;
const auth = openCSGAuthGuardStub(false);
const templates: never[] = [];

function setup(path: string) {
  useWorkspaceUiStore.setState({
    selectedHubResourceType: "template",
    selectedHubTemplateId: "",
    selectedHubSkillName: "",
    selectedHubSkillPath: "",
    selectedMCPServerName: "",
    selectedKnowledgeBaseID: "",
  });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const hook = renderHook(
    ({ path: currentPath }) =>
      useWorkspaceHubSelection({
        activePane: paneFromLocation(currentPath),
        templates,
        loaded: false,
        openCSGAuthGuard: auth,
        t,
      }),
    { wrapper, initialProps: { path } },
  );
  return { ...hook, client };
}

beforeEach(() => {
  vi.mocked(fetchMCPServers).mockReset();
  vi.mocked(fetchMCPServers).mockResolvedValue(catalog);
});

describe("agent MCP catalog loading", () => {
  it("loads the catalog on first DSH agent entry and reports loading until the request settles", async () => {
    let resolveCatalog: ((value: typeof catalog) => void) | undefined;
    vi.mocked(fetchMCPServers).mockImplementationOnce(
      () =>
        new Promise<typeof catalog>((resolve) => {
          resolveCatalog = resolve;
        }),
    );
    const { result, unmount, client } = setup("/agents/generic-assistant-dsh-3");

    expect(fetchMCPServers).toHaveBeenCalledTimes(1);
    expect(result.current.mcpServersLoading).toBe(true);
    expect(result.current.mcpServers).toEqual([]);

    await act(async () => resolveCatalog?.(catalog));
    await waitFor(() => expect(result.current.mcpServers).toHaveLength(2));
    expect(result.current.mcpServersLoading).toBe(false);
    expect(result.current.mcpStateError).toBe("");
    expect(useWorkspaceUiStore.getState().selectedHubResourceType).toBe("template");
    unmount();
    client.clear();
  });

  it("keeps the catalog lazy outside its consumers and loads when navigating to a Codex agent", async () => {
    const { result, rerender, unmount, client } = setup("/rooms/room-1");
    expect(fetchMCPServers).not.toHaveBeenCalled();

    rerender({ path: "/agents/codex" });
    await waitFor(() => expect(result.current.mcpServers).toHaveLength(2));
    expect(fetchMCPServers).toHaveBeenCalledTimes(1);
    expect(useWorkspaceUiStore.getState().selectedHubResourceType).toBe("template");
    unmount();
    client.clear();
  });

  it("continues loading the catalog from a direct MCP resource route", async () => {
    const { result, unmount, client } = setup("/mcp-servers/search");
    await waitFor(() => expect(result.current.mcpServers).toHaveLength(2));
    expect(fetchMCPServers).toHaveBeenCalledTimes(1);
    unmount();
    client.clear();
  });

  it("reports catalog failures on an agent page and allows retry", async () => {
    vi.mocked(fetchMCPServers).mockRejectedValueOnce(new Error("Catalog unavailable"));
    const { result, unmount, client } = setup("/agents/generic-assistant-dsh-3");
    await waitFor(() => expect(result.current.mcpStateError).toBe("Catalog unavailable"));
    expect(result.current.mcpServersLoading).toBe(false);

    await act(async () => {
      await result.current.refetchMCPServers();
    });
    await waitFor(() => expect(result.current.mcpServers).toHaveLength(2));
    expect(result.current.mcpStateError).toBe("");
    expect(fetchMCPServers).toHaveBeenCalledTimes(2);
    unmount();
    client.clear();
  });
});

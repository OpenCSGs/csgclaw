import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { installRemoteMCPServerRequest } from "@/api/mcp";
import { GlobalNoticeProvider } from "@/components/ui";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import { useWorkspaceMCPSelection } from "@/hooks/workspace/useWorkspaceMCPSelection";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import { ConnectorsSection } from "@/pages/SettingsPage/sections/ConnectorsSection";
import { createTranslator } from "@/shared/i18n";
import { openCSGAuthGuardStub } from "../helpers/openCSGAuthGuard";

vi.mock("@/api/mcp", async () => ({
  ...(await vi.importActual<typeof import("@/api/mcp")>("@/api/mcp")),
  installRemoteMCPServerRequest: vi.fn(),
}));
vi.mock("@/components/business/Apps", async () => ({
  ...(await vi.importActual<typeof import("@/components/business/Apps")>("@/components/business/Apps")),
  GlobalAppsPanel: ({ catalogOpen }: { catalogOpen?: boolean }) =>
    catalogOpen ? <div role="dialog" aria-label="Add application connector" /> : null,
}));

const t = createTranslator("zh");
const item = { id: "user:39", name: "kb-tourism", url: "https://example.test/mcp" };
const auth = openCSGAuthGuardStub();
const noop = () => {};

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(workspaceQueryKeys.mcpServers(), { mcpServers: {} });
  function Harness() {
    const mcp = useWorkspaceMCPSelection({
      selectedMCPServerName: "",
      selectedHubResourceType: "mcp",
      setSelectedMCPServerName: noop,
      setSelectedHubResourceType: noop,
      skillCount: 0,
      templateCount: 0,
      enabled: false,
      openCSGAuthGuard: auth,
      t,
    });
    return (
      <WorkspaceControllerProvider
        controller={
          {
            ready: true,
            t,
            sidebarProps: { authStatus: { authenticated: true } },
            hubViewProps: {
              hub: {
                ...mcp,
                remoteMCPServers: [item],
                setRemoteMCPServersEnabled: noop,
                detailPaneProps: { loaded: true, onInstallRemoteMCP: mcp.installRemoteMCPServer },
              },
            },
          } as never
        }
      >
        <ConnectorsSection />
      </WorkspaceControllerProvider>
    );
  }
  render(
    <QueryClientProvider client={client}>
      <GlobalNoticeProvider>
        <Harness />
      </GlobalNoticeProvider>
    </QueryClientProvider>,
  );
  return client;
}

afterEach(() => vi.clearAllMocks());

describe("MCP market installation", () => {
  it("opens the connector catalog from the market Add menu", async () => {
    const client = setup();
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "MCP 市场" }));
    await user.click(screen.getByRole("button", { name: "添加" }));
    await user.click(screen.getByRole("menuitem", { name: "添加连接器" }));
    expect(screen.getByRole("tab", { name: "已安装" })).toHaveAttribute("aria-selected", "true");
    expect(await screen.findByRole("dialog", { name: "Add application connector" })).toBeVisible();
    client.clear();
  });

  it("combines MCP and connector creation under one Add menu", async () => {
    setup();
    const user = userEvent.setup();

    await user.click(screen.getByRole("button", { name: "添加" }));

    expect(screen.getByRole("menuitem", { name: "添加 MCP 服务" })).toBeVisible();
    expect(screen.getByRole("menuitem", { name: "添加连接器" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "添加连接器" })).not.toBeInTheDocument();
  });

  it("shows a direct plus-button failure beside that card and in a named notice", async () => {
    vi.mocked(installRemoteMCPServerRequest).mockRejectedValueOnce({
      status: 502,
      code: "remote_mcp_install_failed",
      message: "Bad gateway",
    });
    const client = setup();
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "MCP 市场" }));
    const button = screen.getByRole("button", { name: "安装" });
    const card = button.closest("article")!;
    await user.click(button);
    expect(await within(card).findByRole("alert")).toHaveTextContent("MCP 安装失败");
    expect(screen.getByText("“kb-tourism”安装失败")).toBeVisible();
    expect(screen.getByRole("tab", { name: "MCP 市场" })).toHaveAttribute("aria-selected", "true");
    expect(button).toBeEnabled();
    client.clear();
  });

  it.each(["list", "detail"])("keeps the market and search after a successful installation from %s", async (entry) => {
    const client = setup();
    vi.mocked(installRemoteMCPServerRequest).mockImplementationOnce(async () => {
      client.setQueryData(workspaceQueryKeys.mcpServers(), {
        mcpServers: {
          mcp_fixed: {
            display_name: item.name,
            url: item.url,
            _meta: { "com.opencsg/marketplace": { server_id: item.id } },
          },
        },
      });
      return "mcp_fixed";
    });
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "MCP 市场" }));
    await user.type(screen.getByRole("searchbox", { name: "搜索 MCP 市场" }), "tourism");
    if (entry === "detail") await user.click(screen.getByRole("button", { name: /kb-tourism/ }));
    await user.click(screen.getByRole("button", { name: "安装" }));
    expect(await screen.findByText("“kb-tourism”已安装，可供智能体启用。")).toBeVisible();
    expect(screen.getByRole("tab", { name: "MCP 市场" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("searchbox", { name: "搜索 MCP 市场" })).toHaveValue("tourism");
    const card = screen.getByRole("button", { name: "替换" }).closest("article")!;
    expect(within(card).getByText("已安装")).toBeVisible();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    client.clear();
  });
});

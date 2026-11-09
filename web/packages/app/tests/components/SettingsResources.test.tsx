import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { WorkspaceControllerProvider } from "@/hooks/workspace";
import { MCPDetailDialog } from "@/pages/SettingsPage/sections/MCPDetailDialog";
import { MCPMarketPane } from "@/pages/SettingsPage/sections/MCPMarketPane";
import { KnowledgeBaseDetailDialog } from "@/pages/SettingsPage/sections/KnowledgeBaseDetailDialog";
import { mcpServersFromMap, type MCPProbeResult } from "@/models/mcp";
import { createTranslator } from "@/shared/i18n";

const t = createTranslator("zh");
const servers = mcpServersFromMap({
  manual: { command: "npx", args: ["local-server"] },
  mcp_fixed: {
    display_name: "已改名的搜索",
    url: "https://example.test/mcp",
    _meta: { "com.opencsg/marketplace": { server_id: "42" } },
  },
  kb_tourism: {
    url: "https://example.test/wiki/mcp",
    _meta: {
      "com.opencsg/mcp": {
        type: "llm_wiki",
        resource_id: "142",
        content_id: "tourism",
        auth_type: "csghub_access_token",
      },
    },
  },
});

function wrapper(onProbeMCP = vi.fn(), mcpProbeError = "") {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <WorkspaceControllerProvider
        controller={
          {
            ready: true,
            t,
            hubViewProps: { hub: { mcpServers: servers, detailPaneProps: { onProbeMCP, mcpProbeError } } },
          } as never
        }
      >
        {children}
      </WorkspaceControllerProvider>
    );
  };
}

describe("settings resource actions", () => {
  it("shows installation progress and failures inside the market detail dialog, then allows retry", async () => {
    const target = { variant: "market" as const, server: { id: "user:39", name: "kb-tourism" } };
    const onAction = vi.fn();
    const view = render(
      <MCPDetailDialog
        target={target}
        t={t}
        onOpenChange={vi.fn()}
        market={{ installed: false, busy: true, onAction }}
      />,
      { wrapper: wrapper() },
    );
    expect(screen.getByRole("button", { name: "安装中..." })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("安装中...");
    view.rerender(
      <MCPDetailDialog
        target={target}
        t={t}
        onOpenChange={vi.fn()}
        market={{ installed: false, busy: false, error: t("resourcesMCPRemoteInstallSignInRequired"), onAction }}
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("请重新登录");
    await userEvent.setup().click(screen.getByRole("button", { name: "安装" }));
    expect(onAction).toHaveBeenCalledOnce();
  });
  it.each([false, true])("recognizes a renamed installation in the market (rows: %s)", async (rows) => {
    const user = userEvent.setup();
    const item = { id: "42", name: "联网搜索" };
    const onInstall = vi.fn();
    render(
      <MCPMarketPane
        authed
        hub={undefined}
        rows={rows}
        installedServers={servers}
        remoteServers={[item]}
        onInstall={onInstall}
        onLogin={vi.fn()}
        onOpenServer={vi.fn()}
        t={t}
      />,
    );
    expect(screen.queryByRole("button", { name: "安装" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "替换" }));
    expect(onInstall).toHaveBeenCalledWith(item);
  });

  it.each(["manual", "knowledge"])("shows failures for %s MCP connections", async (kind) => {
    const onProbeMCP = vi.fn().mockResolvedValue(null);
    render(
      kind === "manual" ? (
        <MCPDetailDialog target={{ variant: "installed", name: "manual" }} t={t} onOpenChange={vi.fn()} />
      ) : (
        <KnowledgeBaseDetailDialog
          target={{
            variant: "installed",
            kb: {
              id: "142",
              contentID: "tourism",
              name: "旅游知识库",
              configuredMCPName: "kb_tourism",
              availability: "available",
            },
          }}
          t={t}
          onOpenChange={vi.fn()}
        />
      ),
      { wrapper: wrapper(onProbeMCP, "服务连接超时") },
    );
    await userEvent.setup().click(screen.getByRole("button", { name: "测试连接" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("服务连接超时");
    expect(onProbeMCP).toHaveBeenCalledWith({
      name: kind === "manual" ? "manual" : "kb_tourism",
      config: servers.find((server) => server.name === (kind === "manual" ? "manual" : "kb_tourism"))!.config,
    });
  });

  it("shows a fallback failure even when the controller returns no diagnostics", async () => {
    render(<MCPDetailDialog target={{ variant: "installed", name: "mcp_fixed" }} t={t} onOpenChange={vi.fn()} />, {
      wrapper: wrapper(vi.fn().mockResolvedValue(null)),
    });
    await userEvent.setup().click(screen.getByRole("button", { name: "测试连接" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(t("resourcesMCPTestFailed"));
    await userEvent.setup().click(within(screen.getByRole("alert")).getByRole("button", { name: "关闭" }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("keeps installed knowledge-base actions visible while its MCP catalog loads", () => {
    render(
      <KnowledgeBaseDetailDialog
        target={{
          variant: "installed",
          kb: {
            id: "999",
            contentID: "not-loaded",
            name: "旅游知识库",
            description: "Long description ".repeat(200),
            availability: "available",
            configuredMCPName: "not-loaded",
          },
        }}
        t={t}
        onOpenChange={vi.fn()}
      />,
      { wrapper: wrapper() },
    );
    expect(screen.getByRole("button", { name: "测试连接" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "删除" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "重试" })).toBeEnabled();
  });

  it("shows pending and successful tool discovery and ignores completion after switching targets", async () => {
    let finish!: (result: MCPProbeResult) => void;
    const onProbeMCP = vi.fn().mockImplementation(
      () =>
        new Promise<MCPProbeResult>((resolve) => {
          finish = resolve;
        }),
    );
    const view = render(
      <MCPDetailDialog target={{ variant: "installed", name: "mcp_fixed" }} t={t} onOpenChange={vi.fn()} />,
      { wrapper: wrapper(onProbeMCP) },
    );
    const result: MCPProbeResult = {
      connected: true,
      durationMs: 21,
      toolsSupported: true,
      truncated: false,
      tools: [{ name: "search", description: "搜索工具" }],
    };
    await userEvent.setup().click(screen.getByRole("button", { name: "测试连接" }));
    expect(screen.getByRole("button", { name: /测试中/ })).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("测试中");
    await act(async () => finish(result));
    expect(await screen.findByText("搜索工具")).toBeVisible();

    await userEvent.setup().click(screen.getByRole("button", { name: "测试连接" }));
    view.rerender(
      <MCPDetailDialog target={{ variant: "installed", name: "kb_tourism" }} t={t} onOpenChange={vi.fn()} />,
    );
    await act(async () => finish(result));
    await waitFor(() => expect(screen.queryByText("搜索工具")).not.toBeInTheDocument());
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});

import { useState } from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AppDefinition, AppInstallation } from "@/api/apps";
import { createTranslator } from "@/shared/i18n";
import { AgentAppsPanel, AppManagedMCPRows } from "./AgentAppsPanel";
import { AppSettingsDialog } from "./AppSettingsDialog";
import { useAgentApps } from "./useAgentApps";

const t = createTranslator("en");
const definition: AppDefinition = {
  app_id: "gitlab",
  name: "GitLab",
  description: "",
  version: "1",
  interface: {},
  config_schema: {},
  auth_methods: ["bearer"],
  oauth_supported: false,
};
const installation: AppInstallation = {
  installation_id: "install-1",
  resource_id: "resource-1",
  resource_enabled: true,
  agent_id: "agent-1",
  app_id: "gitlab",
  name: "Work GitLab",
  enabled: true,
  disconnected: false,
  status: "connected",
  config: { transport: "http", url: "http://localhost:8888/mcp", auth_mode: "bearer" },
  credentials_set: { token: true },
  tools: [{ name: "list_projects", description: "Read projects" }],
  created_at: "",
  updated_at: "",
};

afterEach(() => vi.unstubAllGlobals());

function Harness({ agentID = "agent-1", mode = "apps" }: { agentID?: string; mode?: "apps" | "mcp" }) {
  const controller = useAgentApps(agentID, true);
  const [selectedID, setSelectedID] = useState<string | undefined>();
  if (mode === "mcp") return <AppManagedMCPRows controller={controller} t={t} onSelect={setSelectedID} />;
  return (
    <AgentAppsPanel agentID={agentID} controller={controller} t={t} selectedID={selectedID} onSelect={setSelectedID} />
  );
}

function mockServer(initial: AppInstallation[] = []) {
  let items = initial;
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url);
    if (path === "api/v1/connectors/catalog") return Response.json({ items: [definition] });
    if (path === "api/v1/connectors/resources")
      return Response.json({ items: [{ ...installation, installation_id: "resource-1", agent_id: "", bindings: [] }] });
    if (init?.method === "POST" && path.endsWith("connectors:probe"))
      return Response.json({ connected: true, tools: installation.tools });
    if (init?.method === "POST" && path.endsWith("/disconnect")) {
      items = items.map((app) => ({
        ...app,
        disconnected: true,
        status: "disconnected",
        credentials_set: {},
        tools: [],
      }));
      return Response.json(items[0]);
    }
    if (init?.method === "POST" && path.endsWith("/connectors")) {
      const body = JSON.parse(String(init.body));
      const saved = { ...installation, resource_id: body.resource_id };
      items = [...items, saved];
      return Response.json(saved);
    }
    if (path === "api/v1/agents/agent-1/connectors") return Response.json({ items });
    if (path === "api/v1/agents/agent-2/connectors") return Response.json({ items: [] });
    throw new Error(`Unexpected request ${init?.method || "GET"} ${path}`);
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

describe("Agent Apps", () => {
  it("binds a global resource without asking for or copying credentials", async () => {
    const fetch = mockServer();
    const user = userEvent.setup();
    render(<Harness />);
    await screen.findByText("Connect the services your agent needs");
    await user.click(screen.getByRole("button", { name: "Add from resources" }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Work GitLab" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByText("Work GitLab")).toBeVisible();
    const create = fetch.mock.calls.find(
      ([url, init]) => url === "api/v1/agents/agent-1/connectors" && init?.method === "POST",
    );
    expect(JSON.parse(String(create?.[1]?.body))).toEqual({ resource_id: "resource-1", connect: true });
    expect(screen.queryByLabelText("Token / API key")).not.toBeInTheDocument();
  });

  it("disconnects explicitly and never reconnects when the list is fetched again", async () => {
    const fetch = mockServer([installation]);
    const user = userEvent.setup();
    const view = render(<Harness />);
    await screen.findByText("Work GitLab");
    await user.click(screen.getByRole("button", { name: "Disconnect" }));
    await screen.findByText("Disconnected");
    view.unmount();
    render(<Harness />);
    await screen.findByText("Manually disconnected. Reconnect explicitly to restore access.");
    expect(fetch.mock.calls.some(([url]) => url.endsWith("/connect"))).toBe(false);
  });

  it("clears the previous agent's installations when the selected agent changes", async () => {
    mockServer([installation]);
    const view = render(<Harness />);
    await screen.findByText("Work GitLab");
    view.rerender(<Harness agentID="agent-2" />);
    expect(screen.queryByText("Work GitLab")).not.toBeInTheDocument();
    await screen.findByText("Connect the services your agent needs");
  });

  it("展示 App MCP 开关与详情弹框", async () => {
    mockServer([installation]);
    render(<Harness mode="mcp" />);
    await screen.findByText("Managed by connector · Connected");
    expect(screen.getByRole("switch", { name: "Work GitLab" })).toBeEnabled();
    await userEvent.click(screen.getByRole("button", { name: /Managed by connector/ }));
    expect(screen.getByRole("dialog")).toBeVisible();
    expect(screen.getByRole("button", { name: "Settings" })).toBeVisible();
  });

  it("在 App MCP 详情中显示启用失败", async () => {
    const fetch = mockServer([installation]);
    render(<Harness mode="mcp" />);
    await screen.findByText("Managed by connector · Connected");
    await userEvent.click(screen.getByRole("button", { name: /Managed by connector/ }));
    const dialog = screen.getByRole("dialog");
    fetch.mockResolvedValueOnce(Response.json({ error: "update failed" }, { status: 500 }));
    await userEvent.click(within(dialog).getByRole("switch", { name: "Work GitLab" }));
    expect(await within(dialog).findByRole("alert")).toBeVisible();
  });

  it("requires a fresh connection test after changing settings and never prefills saved secrets", async () => {
    const user = userEvent.setup();
    const probe = vi.fn().mockResolvedValue({ connected: true, tools: installation.tools });
    const wikiDefinition = { ...definition, app_id: "llm-wiki", name: "LLM Wiki" };
    const wikiInstallation = {
      ...installation,
      app_id: "llm-wiki",
      name: "Work Wiki",
      config: { transport: "http" as const, url: "http://localhost:8888/mcp", auth_mode: "bearer" as const },
    };
    render(
      <AppSettingsDialog
        definition={wikiDefinition}
        existing={wikiInstallation}
        t={t}
        onClose={vi.fn()}
        onProbe={probe}
        onSave={vi.fn()}
      />,
    );
    const secret = screen.getByLabelText("Token / API key");
    expect(secret).toHaveValue("");
    expect(secret).toHaveAttribute("placeholder", "••••••••");
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Save and connect" })).toBeEnabled());
    await user.type(screen.getByLabelText("MCP service URL"), "/other");
    expect(screen.getByRole("button", { name: "Save and connect" })).toBeDisabled();
    expect(screen.getByText("Settings changed. Test the connection again.")).toBeVisible();
    expect(probe.mock.calls[0][0].credentials.token).toBeUndefined();
  });

  it("shows the GitLab MCP endpoint and global PAT Connector configuration", async () => {
    const user = userEvent.setup();
    render(
      <AppSettingsDialog
        globalResource
        definition={{ ...definition, auth_methods: ["connector"] }}
        existing={null}
        t={t}
        onClose={vi.fn()}
        onProbe={vi.fn()}
        onSave={vi.fn()}
      />,
    );
    const instanceURL = screen.getByLabelText("GitLab instance URL");
    expect(instanceURL).toHaveAttribute("placeholder", "https://gitlab.example.com");
    await user.type(instanceURL, "https://gitlab.example.com");
    expect(screen.queryByRole("combobox", { name: "Connection type" })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Authentication" })).not.toBeInTheDocument();
    expect(screen.getByLabelText("GitLab Personal Access Token")).toBeVisible();
    expect(screen.getByRole("combobox", { name: "Platform credential source" })).toBeVisible();
    expect(screen.queryByText("OAuth2 authorization is not supported in this version.")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save configuration" })).toBeDisabled();
    await user.type(screen.getByLabelText("MCP service URL"), "http://localhost/mcp");
    expect(screen.getByRole("button", { name: "Save configuration" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Test connection" })).toBeDisabled();
    await user.type(screen.getByLabelText("GitLab Personal Access Token"), "draft-pat");
    expect(screen.getByRole("button", { name: "Test connection" })).toBeEnabled();
  });

  it("uses connector-owned Feishu credentials and keeps local command arguments separate", async () => {
    const user = userEvent.setup();
    const probe = vi.fn().mockResolvedValue({ connected: true, tools: [] });
    render(
      <AppSettingsDialog
        definition={{ ...definition, app_id: "feishu" }}
        existing={null}
        t={t}
        onClose={vi.fn()}
        onProbe={probe}
        onSave={vi.fn()}
      />,
    );
    expect(screen.getByLabelText("App Secret")).toBeInTheDocument();
    await user.type(screen.getByLabelText("App ID"), "test-app");
    await user.type(screen.getByLabelText("App Secret"), "test-secret");
    expect(screen.queryByText("Use this agent’s Feishu channel")).not.toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: "Connection type" }));
    await user.click(screen.getByRole("option", { name: "Local process (stdio)" }));
    await user.type(screen.getByLabelText("Command"), "node");
    await user.type(screen.getByLabelText("Arguments"), "mcp.js\n--label=team bot");
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    await screen.findByText("Connected · 0 tools");
    expect(probe.mock.calls[0][0]).toMatchObject({
      config: {
        transport: "stdio",
        command: "node",
        args: ["mcp.js", "--label=team bot"],
        auth_mode: "feishu",
        credential_source: "manual",
        app_id_env: "FEISHU_APP_ID",
        app_secret_env: "FEISHU_APP_SECRET",
      },
    });
    expect(probe.mock.calls[0][0].credentials.app_secret).toBe("test-secret");
  });
});

describe("Feishu managed authentication", () => {
  it("uses connector credentials and a platform token without requesting header names", async () => {
    const user = userEvent.setup();
    const onProbe = vi.fn().mockResolvedValue({ connected: true, tools: [] });
    render(
      <AppSettingsDialog
        definition={{ ...definition, app_id: "feishu", name: "Feishu" }}
        existing={null}
        t={t}
        onClose={vi.fn()}
        onProbe={onProbe}
        onSave={vi.fn()}
      />,
    );
    expect(screen.queryByLabelText("App ID header name")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("App Secret header name")).not.toBeInTheDocument();
    expect(screen.getByLabelText("App Secret")).toBeInTheDocument();
    await user.type(screen.getByLabelText("App ID"), "test-app");
    await user.type(screen.getByLabelText("App Secret"), "test-secret");
    await user.type(screen.getByLabelText("MCP service URL"), "https://passthrough.example/mcp");
    await user.type(screen.getByLabelText("Platform access token (optional)"), "platform-secret-test");
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    await waitFor(() =>
      expect(onProbe).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({ auth_mode: "feishu", credential_source: "manual" }),
          credentials: expect.objectContaining({ token: "platform-secret-test" }),
        }),
      ),
    );
  });
});

describe("Platform login and connection errors", () => {
  it("reuses OpenCSG login without submitting a copied platform token", async () => {
    const user = userEvent.setup();
    const onProbe = vi.fn().mockResolvedValue({ connected: true, tools: [] });
    render(
      <AppSettingsDialog
        definition={{ ...definition, app_id: "feishu", name: "Feishu" }}
        existing={null}
        t={t}
        onClose={vi.fn()}
        onProbe={onProbe}
        onSave={vi.fn()}
      />,
    );
    await user.type(screen.getByLabelText("App ID"), "test-app");
    await user.type(screen.getByLabelText("App Secret"), "test-secret");
    await user.type(screen.getByLabelText("MCP service URL"), "https://demo.public.opencsg-stg.com/mcp");
    expect(screen.queryByLabelText("Platform access token (optional)")).not.toBeInTheDocument();
    await user.click(screen.getByRole("combobox", { name: "Platform credential source" }));
    await user.click(screen.getByRole("option", { name: "Manual configuration" }));
    await user.type(screen.getByLabelText("Platform access token (optional)"), "stale-platform-secret");
    await user.click(screen.getByRole("combobox", { name: "Platform credential source" }));
    await user.click(screen.getByRole("option", { name: "Use current OpenCSG login" }));
    expect(screen.queryByLabelText("Platform access token (optional)")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    await waitFor(() => expect(onProbe).toHaveBeenCalled());
    const body = onProbe.mock.calls[0][0];
    expect(body.config.platform_credential_source).toBe("opencsg_login");
    expect(body.credentials.token).toBeUndefined();
  });
  it("shows token expiry and upstream status instead of a generic failure", async () => {
    mockServer([
      {
        ...installation,
        status: "authorization_required",
        last_error: "App authorization is no longer valid",
        last_error_code: "app_platform_token_expired",
        last_error_http_status: 401,
      },
    ]);
    render(<Harness />);
    expect(await screen.findByText(/The platform token has expired/)).toHaveTextContent("HTTP 401");
    expect(screen.queryByText("App authorization is no longer valid")).not.toBeInTheDocument();
  });
});

describe("App connection form layout", () => {
  it("separates the MCP endpoint, platform access, and Feishu identity", () => {
    render(
      <AppSettingsDialog
        definition={{ ...definition, app_id: "feishu" }}
        existing={null}
        t={t}
        onClose={vi.fn()}
        onProbe={vi.fn()}
        onSave={vi.fn()}
      />,
    );
    const connection = screen.getByRole("region", { name: "Service connection" });
    const access = screen.getByRole("region", { name: "MCP service authentication" });
    const identity = screen.getByRole("region", { name: "Feishu application identity" });
    expect(within(connection).getByLabelText("MCP service URL")).toBeInTheDocument();
    expect(within(access).getByLabelText("Platform access token (optional)")).toBeInTheDocument();
    expect(within(identity).getByLabelText("App Secret")).toBeInTheDocument();
    expect(within(access).queryByLabelText("App Secret")).not.toBeInTheDocument();
    expect(within(identity).queryByLabelText("Platform access token (optional)")).not.toBeInTheDocument();
  });
});

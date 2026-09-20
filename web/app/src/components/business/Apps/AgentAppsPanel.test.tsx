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
    <AgentAppsPanel
      agentID={agentID}
      controller={controller}
      hasFeishuChannel={false}
      t={t}
      selectedID={selectedID}
      onSelect={setSelectedID}
    />
  );
}

function mockServer(initial: AppInstallation[] = []) {
  let items = initial;
  const fetch = vi.fn(async (url: string, init?: RequestInit) => {
    const path = String(url);
    if (path === "api/v1/apps") return Response.json({ items: [definition] });
    if (path === "api/v1/app-resources")
      return Response.json({ items: [{ ...installation, installation_id: "resource-1", agent_id: "", bindings: [] }] });
    if (init?.method === "POST" && path.endsWith("apps:probe"))
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
    if (init?.method === "POST" && path.endsWith("/apps")) {
      const body = JSON.parse(String(init.body));
      const saved = { ...installation, resource_id: body.resource_id };
      items = [...items, saved];
      return Response.json(saved);
    }
    if (path === "api/v1/agents/agent-1/apps") return Response.json({ items });
    if (path === "api/v1/agents/agent-2/apps") return Response.json({ items: [] });
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
      ([url, init]) => url === "api/v1/agents/agent-1/apps" && init?.method === "POST",
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

  it("projects app-managed MCP rows with only an App settings action", async () => {
    mockServer([installation]);
    render(<Harness mode="mcp" />);
    await screen.findByText("Managed by app · Connected");
    expect(screen.getAllByRole("button").map((button) => button.textContent)).toEqual(["Settings"]);
  });

  it("requires a fresh connection test after changing settings and never prefills saved secrets", async () => {
    const user = userEvent.setup();
    const probe = vi.fn().mockResolvedValue({ connected: true, tools: installation.tools });
    render(
      <AppSettingsDialog
        definition={definition}
        existing={installation}
        hasFeishuChannel={false}
        t={t}
        onClose={vi.fn()}
        onProbe={probe}
        onSave={vi.fn()}
      />,
    );
    const secret = screen.getByLabelText("Token / API key");
    expect(secret).toHaveValue("");
    expect(secret).toHaveAttribute("placeholder", "Configured; leave blank to keep");
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Save and connect" })).toBeEnabled());
    await user.type(screen.getByLabelText("MCP service URL"), "/other");
    expect(screen.getByRole("button", { name: "Save and connect" })).toBeDisabled();
    expect(screen.getByText("Settings changed. Test the connection again.")).toBeVisible();
    expect(probe.mock.calls[0][0].credentials.token).toBeUndefined();
  });

  it("uses the Feishu channel reference and keeps local command arguments separate", async () => {
    const user = userEvent.setup();
    const probe = vi.fn().mockResolvedValue({ connected: true, tools: [] });
    render(
      <AppSettingsDialog
        definition={{ ...definition, app_id: "feishu" }}
        existing={null}
        hasFeishuChannel
        t={t}
        onClose={vi.fn()}
        onProbe={probe}
        onSave={vi.fn()}
      />,
    );
    expect(screen.queryByLabelText("App Secret")).not.toBeInTheDocument();
    expect(screen.getByText("Use this agent’s Feishu channel")).toBeVisible();
    await user.click(screen.getByRole("combobox", { name: "Connection type" }));
    await user.click(screen.getByRole("option", { name: "Local process (stdio)" }));
    await user.type(screen.getByLabelText("Command"), "node");
    await user.type(screen.getByLabelText("Arguments"), "mcp.js\n--label=team bot");
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    await screen.findByText("Connection successful, 0 tools found");
    expect(probe.mock.calls[0][0]).toMatchObject({
      config: {
        transport: "stdio",
        command: "node",
        args: ["mcp.js", "--label=team bot"],
        auth_mode: "feishu",
        credential_source: "feishu_channel",
        app_id_env: "FEISHU_APP_ID",
        app_secret_env: "FEISHU_APP_SECRET",
      },
    });
    expect(probe.mock.calls[0][0].credentials).not.toHaveProperty("app_secret");
  });
});

describe("Feishu managed authentication", () => {
  it("uses channel credentials and a platform token without requesting header names", async () => {
    const user = userEvent.setup();
    const onProbe = vi.fn().mockResolvedValue({ connected: true, tools: [] });
    render(
      <AppSettingsDialog
        definition={{ ...definition, app_id: "feishu", name: "Feishu" }}
        existing={null}
        hasFeishuChannel={true}
        t={t}
        onClose={vi.fn()}
        onProbe={onProbe}
        onSave={vi.fn()}
      />,
    );
    expect(screen.queryByLabelText("App ID header name")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("App Secret header name")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("App Secret")).not.toBeInTheDocument();
    await user.type(screen.getByLabelText("MCP service URL"), "https://passthrough.example/mcp");
    await user.type(screen.getByLabelText("Platform access token (optional)"), "platform-secret-test");
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    await waitFor(() =>
      expect(onProbe).toHaveBeenCalledWith(
        expect.objectContaining({
          config: expect.objectContaining({ auth_mode: "feishu", credential_source: "feishu_channel" }),
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
        hasFeishuChannel={true}
        t={t}
        onClose={vi.fn()}
        onProbe={onProbe}
        onSave={vi.fn()}
      />,
    );
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
        hasFeishuChannel={false}
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

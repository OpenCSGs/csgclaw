import type { ComponentProps } from "react";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Routes, Route, useNavigate } from "react-router-dom";
import { createTranslator } from "@/shared/i18n";
import { GlobalAppsPanel } from "./GlobalAppsPanel";
import { AppSettingsDialog } from "./AppSettingsDialog";
import type { AppDefinition } from "@/api/apps";
const t = createTranslator("en");
const definition: AppDefinition = {
  app_id: "feishu",
  name: "Feishu",
  description: "",
  version: "1",
  interface: {},
  config_schema: {},
  auth_methods: ["feishu"],
  oauth_supported: false,
};
afterEach(() => vi.unstubAllGlobals());

describe("Global Apps", () => {
  it("loads the selected CLI resource when navigating between authorization dialogs", async () => {
    const definitions = ["first", "second"].map((id) => ({
      ...definition,
      app_id: id,
      name: id,
      kind: "cli" as const,
    }));
    const resources = definitions.map((item, index) => ({
      installation_id: item.app_id,
      app_id: item.app_id,
      name: item.name,
      kind: "cli",
      enabled: true,
      authorized: index === 0,
      status: index === 0 ? "connected" : "needs_configuration",
      config: {},
      credentials_set: {},
      tools: [],
    }));
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => {
        if (String(url).endsWith("catalog")) return Response.json({ items: definitions });
        const resource = resources.find((item) => String(url).endsWith(`/resources/${item.installation_id}`));
        return Response.json(resource ?? { items: resources });
      }),
    );
    let navigate!: ReturnType<typeof useNavigate>;
    function Harness() {
      navigate = useNavigate();
      return <GlobalAppsPanel t={t} />;
    }
    render(
      <MemoryRouter initialEntries={["/connectors/first"]}>
        <Routes>
          <Route path="/connectors/:resourceId" element={<Harness />} />
        </Routes>
      </MemoryRouter>,
    );
    const first = within(await screen.findByRole("dialog"));
    expect(first.getByRole("button", { name: "Disconnect" })).toBeVisible();
    await act(async () => navigate("/connectors/second"));
    const second = within(await screen.findByRole("dialog", { name: "second" }));
    expect(await second.findByRole("button", { name: "Connect" })).toBeVisible();
    expect(second.queryByRole("button", { name: "Disconnect" })).not.toBeInTheDocument();
  });

  it("replaces a saved environment error with the current successful probe", async () => {
    const user = userEvent.setup();
    const resource = {
      installation_id: "global-1",
      app_id: "feishu",
      name: "Team Feishu",
      feishu_app_id: "test-app",
      enabled: true,
      status: "error",
      last_error: "Environment mismatch",
      last_error_code: "app_opencsg_environment_mismatch",
      config: { transport: "http", url: "https://service.example/mcp", auth_mode: "feishu" },
      credentials_set: { app_id: true, app_secret: true },
      tools: [],
      bindings: [],
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === "POST") return Response.json({ connected: true, tools: [] });
        return Response.json({ items: String(url).endsWith("catalog") ? [definition] : [resource] });
      }),
    );
    render(
      <MemoryRouter initialEntries={["/connectors/global-1"]}>
        <Routes>
          <Route path="/connectors/:resourceId" element={<GlobalAppsPanel t={t} />} />
        </Routes>
      </MemoryRouter>,
    );
    const dialog = within(await screen.findByRole("dialog"));
    expect(dialog.getByRole("alert")).toHaveTextContent(t("errors.app_opencsg_environment_mismatch"));
    await user.click(dialog.getByRole("button", { name: "Test connection" }));
    expect(await dialog.findByRole("status")).toHaveTextContent("Connected · 0 tools");
    expect(dialog.queryByRole("alert")).not.toBeInTheDocument();
    expect(dialog.queryByText(t("appStatusError"))).not.toBeInTheDocument();
    expect(dialog.getByText(t("appResourceUsers", { count: 0 }))).toBeVisible();
    await user.type(dialog.getByLabelText("MCP service URL"), "/changed");
    expect(dialog.queryByRole("status")).not.toBeInTheDocument();
    expect(dialog.getByRole("alert")).toHaveTextContent(t("errors.app_opencsg_environment_mismatch"));
  });

  it("tests the current draft without saving and invalidates the result after edits", async () => {
    const user = userEvent.setup();
    // These assertions exercise draft changes, not individual keystrokes.
    const enter = async (label: string, value: string) => {
      await user.click(screen.getByLabelText(label));
      await user.paste(value);
    };
    const close = vi.fn();
    const probe = vi.fn().mockResolvedValue({
      connected: true,
      tools: [{ name: "inspect", description: "Long tool details should not appear" }],
    });
    let reject: (e: Error) => void = () => {};
    const save = vi.fn(
      (_payload: Parameters<NonNullable<ComponentProps<typeof AppSettingsDialog>["onSave"]>>[0]) =>
        new Promise<void>((_, fail) => {
          reject = fail;
        }),
    );
    render(
      <AppSettingsDialog
        globalResource
        definition={definition}
        existing={null}
        t={t}
        onClose={close}
        onProbe={probe}
        onSave={save}
      />,
    );
    expect(screen.queryByRole("combobox", { name: "Credential source" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Test connection" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Save configuration" })).toBeDisabled();
    await enter("MCP service URL", "https://service.example/mcp");
    await enter("App ID", "test-app");
    expect(screen.getByLabelText("App Secret")).toBeRequired();
    expect(screen.getByLabelText("App Secret").closest("label")?.querySelector(".field-required")).toHaveTextContent(
      "*",
    );
    expect(screen.getByRole("button", { name: "Test connection" })).toBeDisabled();
    await enter("App Secret", "   ");
    expect(screen.getByRole("button", { name: "Save configuration" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Test connection" })).toBeDisabled();
    await user.clear(screen.getByLabelText("App Secret"));
    await enter("App Secret", "test-secret");
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Connected · 1 tools");
    expect(screen.queryByText("Current settings tested successfully. Not saved yet.")).not.toBeInTheDocument();
    expect(screen.getByText("Long tool details should not appear")).not.toBeVisible();
    const tools = screen.getByText("Available tools (1)").closest("details");
    expect(tools).not.toHaveAttribute("open");
    await user.click(screen.getByText("Available tools (1)"));
    expect(screen.getByText("Long tool details should not appear")).toBeVisible();
    expect(probe.mock.calls[0][0].credentials.app_secret).toBe("test-secret");
    expect(save).not.toHaveBeenCalled();
    await user.clear(screen.getByLabelText("App Secret"));
    expect(screen.getByRole("button", { name: "Save configuration" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Test connection" })).toBeDisabled();
    await enter("App Secret", "test-secret-changed");
    expect(screen.getByRole("button", { name: "Save configuration" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    expect(probe.mock.calls[1][0].credentials.app_secret).toBe("test-secret-changed");
    await user.click(screen.getByRole("button", { name: "Save configuration" }));
    expect(screen.getByRole("button", { name: "Save configuration" })).toBeDisabled();
    expect(close).not.toHaveBeenCalled();
    reject(new Error("Connection validation failed"));
    expect(await screen.findByRole("alert")).toHaveTextContent("Connection validation failed");
    expect(screen.getByLabelText("App ID")).toHaveValue("test-app");
    expect(close).not.toHaveBeenCalled();
    expect(save.mock.calls[0][0].config.credential_source).toBe("manual");
  });
  it("opens global connector settings when its card is activated", async () => {
    const resource = {
      installation_id: "global-1",
      resource_id: "global-1",
      agent_id: "",
      app_id: "feishu",
      name: "Team Feishu",
      enabled: true,
      resource_enabled: true,
      status: "configured",
      config: {},
      credentials_set: {},
      tools: [],
      bindings: [
        {
          agent_id: "agent-manager",
          agent_name: "Manager",
          installation_id: "binding-1",
          enabled: true,
          status: "connected",
        },
      ],
    };
    const fetch = vi.fn(async (url: string) => {
      if (String(url) === "api/v1/connectors/catalog") return Response.json({ items: [definition] });
      return Response.json({ items: [resource] });
    });
    vi.stubGlobal("fetch", fetch);
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={["/connectors"]}>
        <Routes>
          <Route path="/connectors" element={<GlobalAppsPanel t={t} />} />
          <Route path="/connectors/:resourceId" element={<GlobalAppsPanel t={t} />} />
        </Routes>
      </MemoryRouter>,
    );
    await screen.findByText("Team Feishu");
    await user.click(screen.getByRole("button", { name: "Actions for Team Feishu" }));
    expect(screen.getByRole("menuitem", { name: "Settings" })).toBeVisible();
    expect(screen.getByRole("menuitem", { name: "Disable" })).toBeVisible();
    expect(screen.getByRole("menuitem", { name: "Remove" })).toBeVisible();
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: /^Team Feishu/ }));
    expect(screen.getByRole("dialog")).toHaveTextContent("Team Feishu");
  });
});

describe("Connector creation and existing forms", () => {
  it("offers application types and keeps connection options and advanced fields available", async () => {
    const gitlab = {
      ...definition,
      app_id: "gitlab",
      name: "GitLab",
      config_schema: { properties: { url: { default: "https://service.example/mcp" } } },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) =>
        Response.json({
          items: String(url).endsWith("catalog")
            ? [gitlab, definition, { ...definition, app_id: "llm-wiki", name: "LLM Wiki" }]
            : [],
        }),
      ),
    );
    const user = userEvent.setup();
    render(
      <MemoryRouter>
        <GlobalAppsPanel t={t} />
      </MemoryRouter>,
    );
    await screen.findByText("Connect the services your agent needs");
    expect(screen.queryByRole("button", { name: /GitLab/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add connector" }));
    const picker = screen.getByRole("dialog");
    expect(within(picker).getByRole("heading", { name: "Add application connector" })).toBeVisible();
    expect(within(picker).queryByText("LLM Wiki")).not.toBeInTheDocument();
    await user.click(within(picker).getByRole("button", { name: /GitLab/ }));
    expect(screen.getByLabelText("GitLab instance URL")).toBeVisible();
    expect(screen.getByLabelText("GitLab Personal Access Token")).toBeVisible();
    expect(screen.getByLabelText("Instance name")).toBeVisible();
    expect(screen.getByLabelText("MCP service URL")).toHaveValue("https://service.example/mcp");
    expect(screen.getByRole("combobox", { name: "Platform credential source" })).toBeVisible();
    const name = screen.getByLabelText("Instance name");
    const service = screen.getByLabelText("MCP service URL");
    const credentials = screen.getByRole("combobox", { name: "Platform credential source" });
    const instance = screen.getByLabelText("GitLab instance URL");
    expect(name.closest("details")).toBeNull();
    expect(service.compareDocumentPosition(instance) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(credentials.compareDocumentPosition(instance) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await user.click(screen.getByText("Advanced connection settings"));
    expect(screen.getByLabelText("Connection timeout (seconds)")).toBeVisible();
    expect(screen.getByRole("button", { name: "Add field" })).toBeVisible();
    const token = screen.getByLabelText("GitLab Personal Access Token");
    await user.type(token, "fixture-token");
    await user.click(within(token.closest("label")!).getByRole("button", { name: "Show credential" }));
    expect(token).toHaveAttribute("type", "text");
    await user.click(within(token.closest("label")!).getByRole("button", { name: "Hide credential" }));
    expect(token).toHaveAttribute("type", "password");
  });
});

it("shows configured instance names and leaves application types in the add picker", async () => {
  const resources = ["Company workspace", "Personal workspace"].map((name, index) => ({
    installation_id: `resource-${index}`,
    app_id: "feishu",
    name,
    enabled: true,
    status: "configured",
    config: {},
    tools: [],
    credentials_set: {},
    bindings: [],
  }));
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) =>
      Response.json({
        items: String(url).endsWith("catalog")
          ? [definition, { ...definition, app_id: "gitlab", name: "GitLab" }]
          : resources,
      }),
    ),
  );
  render(
    <MemoryRouter>
      <GlobalAppsPanel t={t} />
    </MemoryRouter>,
  );
  expect(await screen.findByRole("button", { name: /^Company workspace/ })).toBeVisible();
  expect(screen.getByRole("button", { name: /^Personal workspace/ })).toBeVisible();
  expect(screen.getByText("Company workspace")).toHaveAttribute("title", "Company workspace");
  expect(screen.queryByRole("button", { name: /^GitLab / })).not.toBeInTheDocument();
});

it("shows live connection states and the actual failure in global connector cards", async () => {
  const resources = [
    { name: "Healthy", status: "connected" },
    {
      name: "Failed",
      status: "error",
      last_error: "MCP connection closed; reconnect the App",
      last_error_code: "app_mcp_connection_closed",
    },
    {
      name: "Connecting",
      status: "connecting",
      last_error: "Stale error from a previous attempt",
    },
    { name: "Unused", status: "not_connected" },
  ].map((item, index) => ({
    ...item,
    installation_id: `live-${index}`,
    app_id: "feishu",
    enabled: true,
    config: {},
    tools: [],
    credentials_set: {},
    bindings: [],
  }));
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => Response.json({ items: String(url).endsWith("catalog") ? [definition] : resources })),
  );
  render(
    <MemoryRouter>
      <GlobalAppsPanel t={t} />
    </MemoryRouter>,
  );
  expect(await screen.findByText("Connected")).toBeVisible();
  expect(screen.getByText(t("appStatusError"))).toBeVisible();
  expect(screen.getByText("Not connected")).toBeVisible();
  expect(screen.getByText(t("errors.app_mcp_connection_closed"))).toBeVisible();
  expect(screen.getAllByText(t("appStatusConnecting")).length).toBeGreaterThanOrEqual(2);
  expect(screen.queryByText("Stale error from a previous attempt")).not.toBeInTheDocument();
  expect(screen.queryByText("Configured")).not.toBeInTheDocument();
});

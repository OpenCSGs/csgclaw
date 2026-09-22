import type { ComponentProps } from "react";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Routes, Route } from "react-router-dom";
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
  it("tests the current draft without saving and invalidates the result after edits", async () => {
    const user = userEvent.setup();
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
    await user.type(screen.getByLabelText("MCP service URL"), "https://service.example/mcp");
    await user.type(screen.getByLabelText("App ID"), "test-app");
    expect(screen.getByLabelText("App Secret")).toBeRequired();
    expect(screen.getByLabelText("App Secret").closest("label")?.querySelector(".field-required")).toHaveTextContent(
      "*",
    );
    expect(screen.getByRole("button", { name: "Test connection" })).toBeDisabled();
    await user.type(screen.getByLabelText("App Secret"), "   ");
    expect(screen.getByRole("button", { name: "Save configuration" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Test connection" })).toBeDisabled();
    await user.clear(screen.getByLabelText("App Secret"));
    await user.type(screen.getByLabelText("App Secret"), "test-secret");
    await user.click(screen.getByRole("button", { name: "Test connection" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Connected · 1 tools");
    expect(screen.getByRole("status")).toHaveTextContent("Not saved yet");
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
    await user.type(screen.getByLabelText("App Secret"), "test-secret-changed");
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
  it("shows affected agents before deleting a global resource", async () => {
    let removed = false;
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
    const fetch = vi.fn(async (url: string, init?: RequestInit) => {
      if (String(url) === "api/v1/connectors/catalog") return Response.json({ items: [definition] });
      if (init?.method === "DELETE") {
        removed = true;
        return new Response(null, { status: 204 });
      }
      return Response.json({ items: removed ? [] : [resource] });
    });
    vi.stubGlobal("fetch", fetch);
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={["/connectors"]}>
        <Routes>
          <Route path="/connectors" element={<GlobalAppsPanel t={t} />} />
        </Routes>
      </MemoryRouter>,
    );
    await screen.findByText("Team Feishu");
    await user.click(screen.getByRole("button", { name: "Remove" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Manager")).toBeVisible();
    expect(removed).toBe(false);
    await user.click(within(dialog).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(removed).toBe(true));
  });
});

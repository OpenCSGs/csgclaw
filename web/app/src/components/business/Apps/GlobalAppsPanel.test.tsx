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
  it("saves a channel reference for later Agent-specific authorization", async () => {
    const user = userEvent.setup();
    const save = vi.fn().mockResolvedValue(undefined);
    render(
      <AppSettingsDialog
        globalResource
        definition={definition}
        existing={null}
        hasFeishuChannel={false}
        t={t}
        onClose={vi.fn()}
        onProbe={vi.fn()}
        onSave={save}
      />,
    );
    expect(screen.getByRole("button", { name: "Test connection" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save configuration" })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: "Save configuration" }));
    await waitFor(() => expect(save).toHaveBeenCalled());
    expect(save.mock.calls[0][0].config.credential_source).toBe("feishu_channel");
    expect(save.mock.calls[0][0].credentials).not.toHaveProperty("app_secret");
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
      status: "agent_identity_required",
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
      if (String(url) === "api/v1/apps") return Response.json({ items: [definition] });
      if (init?.method === "DELETE") {
        removed = true;
        return new Response(null, { status: 204 });
      }
      return Response.json({ items: removed ? [] : [resource] });
    });
    vi.stubGlobal("fetch", fetch);
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={["/apps"]}>
        <Routes>
          <Route path="/apps" element={<GlobalAppsPanel t={t} />} />
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

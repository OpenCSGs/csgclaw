import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { agentToDraft, type AgentLike } from "@/models/agents";
import { createTranslator } from "@/shared/i18n";
import { AgentPage } from "./AgentPage";

const mocked = vi.hoisted(() => ({ controller: {} as Record<string, unknown> }));
vi.mock("@/hooks/workspace", () => ({ useWorkspaceControllerContext: () => mocked.controller }));

function Location() {
  const location = useLocation();
  return (
    <output data-testid="location">
      {location.pathname}
      {location.search}
    </output>
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("Agent App deep links", () => {
  it("opens the requested App configuration directly and clears add_app when closed", async () => {
    const agent: AgentLike = { id: "agent-1", name: "Assistant", runtime_kind: "codex", role: "assistant" };
    mocked.controller = {
      ready: true,
      agentViewProps: {
        item: agent,
        draft: agentToDraft(agent),
        t: createTranslator("en"),
        onDelete: vi.fn(),
        onInvite: vi.fn(),
        onOpenDM: vi.fn(),
        onRecreate: vi.fn(),
        onStart: vi.fn(),
        onStop: vi.fn(),
      },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) =>
        Response.json({
          items:
            String(url) === "api/v1/apps"
              ? [
                  {
                    app_id: "gitlab",
                    name: "GitLab",
                    description: "",
                    version: "1",
                    interface: { displayName: "GitLab" },
                    config_schema: {},
                    auth_methods: ["bearer"],
                    oauth_supported: false,
                  },
                ]
              : [],
        }),
      ),
    );
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={["/agents/agent-1?tab=apps&add_app=gitlab"]}>
        <AgentPage />
        <Location />
      </MemoryRouter>,
    );
    expect(await screen.findByRole("dialog", { name: "Add GitLab" })).toBeVisible();
    expect(screen.getByLabelText("Instance name")).toHaveValue("GitLab");
    expect(
      screen.queryByText("Choose a service for this agent. You can add multiple instances of the same app."),
    ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByTestId("location")).toHaveTextContent("/agents/agent-1?tab=apps");
    expect(screen.getByTestId("location")).not.toHaveTextContent("add_app");
  });
});

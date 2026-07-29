import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { SessionDemoPage } from "@/pages/SessionDemoPage";
import { SESSION_DEMO_STORAGE_KEY } from "@/shared/storage/keys";

function CurrentLocation() {
  return <output data-testid="current-location">{useLocation().search}</output>;
}

describe("SessionDemoPage", () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() => ({
        matches: true,
        media: "(prefers-color-scheme: dark)",
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("creates a live session turn and persists the visible transcript", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify([
            {
              id: "agent-builder",
              name: "Builder",
              participants: [
                {
                  channel: "csgclaw",
                  type: "agent",
                  lifecycle_status: "active",
                  channel_user_kind: "local_user_id",
                  channel_user_ref: "user-builder",
                },
              ],
            },
            {
              id: "agent-reviewer",
              name: "Reviewer",
              participants: [
                {
                  channel: "csgclaw",
                  type: "agent",
                  lifecycle_status: "active",
                  channel_user_kind: "local_user_id",
                  channel_user_ref: "user-reviewer",
                },
              ],
            },
          ]),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "resp-review",
            object: "response",
            created_at: 1,
            completed_at: 2,
            status: "completed",
            model: "agent-reviewer",
            output: [
              {
                id: "msg-review",
                type: "message",
                status: "completed",
                role: "assistant",
                content: [{ type: "output_text", text: "Review complete", annotations: [] }],
              },
            ],
            metadata: { agent_id: "agent-reviewer", room_id: "room-review", session_id: "session-test" },
          }),
          { status: 200 },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={["/session-demo?agent=reviewer&session=session-test"]}>
        <SessionDemoPage />
        <CurrentLocation />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "Reviewer" })).toBeInTheDocument();
    expect(screen.getByText("Anonymous sends as admin")).toBeInTheDocument();
    expect(screen.getByText(/Anonymous Session: session-test \| Agent: Reviewer/)).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByTestId("current-location")).toHaveTextContent("agent=Reviewer");
    });

    await user.type(screen.getByRole("textbox", { name: "Message" }), "Inspect the API");
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("Review complete")).toBeInTheDocument();
    expect(screen.getByText("Inspect the API")).toBeInTheDocument();
    expect(screen.getByText("Room created: room-review")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "api/v1/agents/Reviewer/sessions/session-test/responses",
      expect.objectContaining({ body: JSON.stringify({ input: "Inspect the API" }), method: "POST" }),
    );
    await waitFor(() => {
      expect(window.localStorage.getItem(SESSION_DEMO_STORAGE_KEY)).toContain("Review complete");
    });
    expect(screen.getByText("The session is bound to this agent after its first successful turn.")).toBeInTheDocument();
  });
});

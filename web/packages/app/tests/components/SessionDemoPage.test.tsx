import { act, render, screen, waitFor } from "@testing-library/react";
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
    const encoder = new TextEncoder();
    let finishStream: (() => void) | undefined;
    const responseStream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode(
            'event: message_start\ndata: {"type":"message_start","message":{"id":"resp-review"}}\n\n' +
              'event: content_block_delta\ndata: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Review"}}\n\n',
          ),
        );
        finishStream = () => {
          controller.enqueue(
            encoder.encode(
              'event: content_block_delta\ndata: {"type":"content_block_delta","delta":{"type":"text_delta","text":" complete"}}\n\n' +
                'event: message_stop\ndata: {"type":"message_stop"}\n\n',
            ),
          );
          controller.close();
        };
      },
    });
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify([
            {
              id: "agent-builder",
              name: "Builder",
              runtime: { name: "codex", state: "running" },
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
              runtime: { name: "codex", state: "running" },
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
        new Response(responseStream, { status: 200, headers: { "Content-Type": "text/event-stream" } }),
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
    expect(screen.getByText("Direct Agent Engine execution")).toBeInTheDocument();
    expect(screen.getByText(/Anonymous Session: session-test \| Agent: Reviewer/)).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByTestId("current-location")).toHaveTextContent("agent=Reviewer");
    });

    await user.type(screen.getByRole("textbox", { name: "Message" }), "Inspect the API");
    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("Review")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
    await act(async () => finishStream?.());
    expect(await screen.findByText("Review complete")).toBeInTheDocument();
    expect(screen.getByText("Inspect the API")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "api/v1/agents/Reviewer/sessions/session-test/responses",
      expect.objectContaining({ body: JSON.stringify({ input: "Inspect the API", stream: true }), method: "POST" }),
    );
    await waitFor(() => {
      expect(window.localStorage.getItem(SESSION_DEMO_STORAGE_KEY)).toContain("Review complete");
    });
    expect(screen.getByText("The session is bound to this agent after its first successful turn.")).toBeInTheDocument();
  });

  it("confirms backend cancellation before enabling the next send", async () => {
    const encoder = new TextEncoder();
    const completedStream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode(
            'event: message_start\ndata: {"type":"message_start","message":{"id":"resp-ready"}}\n\n' +
              'event: content_block_delta\ndata: {"type":"content_block_delta","delta":{"type":"text_delta","text":"READY"}}\n\n' +
              'event: message_stop\ndata: {"type":"message_stop"}\n\n',
          ),
        );
        controller.close();
      },
    });
    let confirmCancellation: (() => void) | undefined;
    let finishCanceledResponse: (() => void) | undefined;
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify([{ id: "agent-reviewer", name: "Reviewer", runtime: { name: "codex", state: "running" } }]),
          { status: 200 },
        ),
      )
      .mockImplementationOnce(
        () =>
          new Promise<Response>((_resolve, reject) => {
            finishCanceledResponse = () => reject(new Error("Session stream ended before completion"));
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise<Response>((resolve) => {
            confirmCancellation = () => {
              finishCanceledResponse?.();
              resolve(new Response(null, { status: 204 }));
            };
          }),
      )
      .mockResolvedValueOnce(
        new Response(completedStream, { status: 200, headers: { "Content-Type": "text/event-stream" } }),
      );
    vi.stubGlobal("fetch", fetchMock);

    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={["/session-demo?agent=reviewer&session=session-stop"]}>
        <SessionDemoPage />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "Reviewer" })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Message" }), "Keep working");
    await user.click(screen.getByRole("button", { name: "Send" }));
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("textbox", { name: "Message" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Canceling…" })).toBeDisabled();

    await act(async () => confirmCancellation?.());
    expect(await screen.findByText("Request canceled. You can send it again.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("READY")).toBeInTheDocument();
    expect(screen.queryByText("another response is already running")).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "api/v1/agents/Reviewer/sessions/session-stop/responses/cancel",
      expect.objectContaining({ method: "POST" }),
    );
    expect(fetchMock).toHaveBeenCalledTimes(4);
  });
});

import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { ConversationActivityPanel } from "./ConversationActivityPanel";

vi.mock("@/api/im", () => ({ fetchMessagesRequest: vi.fn(async () => []) }));
vi.mock("@/shared/realtime/imEvents", () => ({ subscribeIMEvents: () => () => undefined }));

describe("member activity selection", () => {
  it("opens the chosen member and reapplies an explicit choice after manual filtering", async () => {
    const user = userEvent.setup();
    const props = {
      agents: [
        { id: "dev", name: "Developer" },
        { id: "test", name: "Tester" },
      ],
      conversation: {
        id: "room",
        members: ["dev", "test"],
        messages: [
          { id: "dev-message", sender_id: "dev", content: "Developer note", created_at: "2026-11-11T12:00:00Z" },
          { id: "test-message", sender_id: "test", content: "Tester note", created_at: "2026-11-11T12:01:00Z" },
        ],
      },
      usersById: new Map(),
      locale: "en" as const,
      t: createTranslator("en"),
      onClose: vi.fn(),
    };
    const { rerender } = render(<ConversationActivityPanel {...props} actorSelection={{ agentID: "dev" }} />);
    expect(screen.getByRole("button", { name: "Developer" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByText("Tester note")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Tester" }));
    expect(screen.queryByText("Developer note")).not.toBeInTheDocument();
    rerender(<ConversationActivityPanel {...props} actorSelection={{ agentID: "dev" }} />);
    expect(screen.getByRole("button", { name: "Developer" })).toHaveAttribute("aria-pressed", "true");
    rerender(<ConversationActivityPanel {...props} actorSelection={{}} />);
    expect(screen.getByText("Developer note")).toBeVisible();
    expect(screen.getByText("Tester note")).toBeVisible();
  });
});

function turnPanelProps() {
  const run = (id: string, actor: string, count: number) => ({
    id,
    sender_id: actor,
    content: "",
    created_at: "2026-11-12T01:01:00Z",
    metadata: {
      codex: { request_id: "request" },
      csgclaw: {
        turn_progress: {
          id,
          revision: 1,
          status: "succeeded",
          started_at: "2026-11-12T01:01:00Z",
          ended_at: "2026-11-12T01:02:00Z",
          items: Array.from({ length: count }, (_, i) => ({
            id: `${id}-${i}`,
            kind: "tool",
            tool: {
              name: "execute",
              status: "completed",
              actions: [],
              command: `${id}-command-${i}`,
              input: `${id}-input-${i}`,
              output: `${id}-output-${i}`,
            },
          })),
        },
      },
    },
  });
  return {
    agents: [
      { id: "dev", name: "Developer" },
      { id: "test", name: "Tester" },
    ],
    conversation: {
      id: "room",
      members: ["dev", "test"],
      messages: [
        { id: "request", sender_id: "admin", content: "Verify the results" },
        run("dev-turn", "dev", 50),
        run("test-turn", "test", 50),
      ],
    },
    usersById: new Map(),
    locale: "en" as const,
    t: createTranslator("en"),
    onClose: vi.fn(),
  };
}
describe("collaboration activity turns", () => {
  it("narrows 100 calls to two executions and preserves the complete selected execution after searching", async () => {
    const user = userEvent.setup();
    render(<ConversationActivityPanel {...turnPanelProps()} />);
    expect(await screen.findByRole("button", { name: "Developer · Turn 1" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Tester · Turn 2" })).toBeVisible();
    expect(screen.queryByText(/dev-turn-command-0$/)).not.toBeInTheDocument();
    const search = screen.getByRole("textbox", { name: "Search triggers, tools or output" });
    await user.type(search, "dev-turn-output-49");
    expect(screen.queryByRole("button", { name: "Tester · Turn 2" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Developer · Turn 1" }));
    const records = screen.getByRole("list", { name: "This turn · Execution order" });
    expect(within(records).getByText(/dev-turn-command-0$/)).toBeVisible();
    expect(within(records).getByText(/dev-turn-command-49$/)).toBeVisible();
    expect(screen.queryByText(/test-turn-command-0$/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Expand all" }));
    expect(within(records).getByText("dev-turn-output-0")).toBeVisible();
    expect(within(records).getByText("dev-turn-input-49")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Back to turns" }));
    expect(screen.getByRole("textbox", { name: "Search triggers, tools or output" })).toHaveValue("dev-turn-output-49");
  });
  it("opens the exact member turn from the working dock", async () => {
    render(
      <ConversationActivityPanel {...turnPanelProps()} actorSelection={{ agentID: "test", turnID: "test-turn" }} />,
    );
    expect(await screen.findByRole("heading", { name: "Tester · Turn 2" })).toBeVisible();
    expect(screen.queryByText(/dev-turn-command-0$/)).not.toBeInTheDocument();
  });
  it("keeps the single-agent activity view unchanged", () => {
    const props = turnPanelProps();
    render(<ConversationActivityPanel {...props} conversation={{ ...props.conversation, is_direct: true }} />);
    expect(screen.queryByRole("textbox", { name: "Search triggers, tools or output" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Developer · Turn 1" })).not.toBeInTheDocument();
  });
});

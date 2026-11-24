import { act, fireEvent, render, renderHook, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchMessagePageRequest } from "@/api/im";
import { subscribeIMEvents } from "@/shared/realtime/imEvents";
import { createTranslator } from "@/shared/i18n";
import { ConversationActivityPanel } from "./ConversationActivityPanel";
import { useActivityHistory } from "./useActivityHistory";

vi.mock("@/api/im", () => ({ fetchMessagePageRequest: vi.fn() }));
vi.mock("@/shared/realtime/imEvents", () => ({ subscribeIMEvents: vi.fn(() => vi.fn()) }));

const fetch = vi.mocked(fetchMessagePageRequest);
beforeEach(() => fetch.mockReset());

function scrollTowardEnd(list: HTMLElement, top: number) {
  const scroller = list.parentElement;
  if (!scroller) throw new Error("Expected a scrollable activity list");
  Object.defineProperties(scroller, {
    clientHeight: { configurable: true, value: 300 },
    scrollHeight: { configurable: true, value: 600 },
  });
  fireEvent.scroll(scroller, { target: { scrollTop: top } });
}

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
    fetch.mockResolvedValue({ messages: props.conversation.messages, has_more: false });
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

function turnPanelProps(toolCount = 50) {
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
  const props = {
    agents: [
      { id: "dev", name: "Developer" },
      { id: "test", name: "Tester" },
    ],
    conversation: {
      id: "room",
      members: ["dev", "test"],
      messages: [
        { id: "request", sender_id: "admin", content: "Verify the results" },
        run("dev-turn", "dev", toolCount),
        run("test-turn", "test", 50),
      ],
    },
    usersById: new Map(),
    locale: "en" as const,
    t: createTranslator("en"),
    onClose: vi.fn(),
  };
  fetch.mockResolvedValue({ messages: props.conversation.messages, has_more: false });
  return props;
}
describe("collaboration activity turns", () => {
  it("narrows 100 calls to two executions and preserves the complete selected execution after searching", async () => {
    const user = userEvent.setup();
    const props = turnPanelProps();
    fetch.mockResolvedValue({ messages: props.conversation.messages, has_more: true });
    render(<ConversationActivityPanel {...props} />);
    expect(await screen.findByRole("button", { name: "Developer · Turn 1" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Tester · Turn 2" })).toBeVisible();
    expect(screen.queryByText(/dev-turn-command-0$/)).not.toBeInTheDocument();
    const search = screen.getByRole("textbox", { name: "Search triggers, tools or output" });
    await user.type(search, "dev-turn-output-49");
    expect(screen.queryByRole("button", { name: "Tester · Turn 2" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Developer · Turn 1" }));
    const records = screen.getByRole("list", { name: "This turn · Execution order" });
    scrollTowardEnd(records, 300);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(within(records).getByText(/dev-turn-command-0$/)).toBeVisible();
    expect(within(records).getByText(/dev-turn-command-49$/)).toBeVisible();
    expect(screen.queryByText(/test-turn-command-0$/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load earlier activity" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Next page" })).not.toBeInTheDocument();
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

it("pauses a hidden activity tab and reconciles thread replies when returning to the selected turn", async () => {
  const user = userEvent.setup();
  const props = turnPanelProps();
  const subscribe = vi.mocked(subscribeIMEvents).mockClear();
  const view = render(<ConversationActivityPanel {...props} />);
  await user.click(await screen.findByRole("button", { name: "Developer · Turn 1" }));
  await user.click(screen.getByRole("button", { name: "Expand all" }));
  const unsubscribe = subscribe.mock.results[0].value;
  view.rerender(<ConversationActivityPanel {...props} active={false} />);
  expect(unsubscribe).toHaveBeenCalledOnce();
  view.rerender(
    <ConversationActivityPanel
      {...props}
      active={false}
      conversation={{ ...props.conversation, messages: [...props.conversation.messages] }}
    />,
  );
  expect(fetch).toHaveBeenCalledTimes(1);
  view.rerender(<ConversationActivityPanel {...props} />);
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  expect(fetch).toHaveBeenLastCalledWith("room", expect.objectContaining({ limit: 100 }));
  expect(subscribe).toHaveBeenCalledTimes(2);
  expect(screen.getByRole("heading", { name: "Developer · Turn 1" })).toBeVisible();
  expect(screen.getByText("dev-turn-output-49")).toBeVisible();
});

it("keeps a 4,146-tool turn navigable with at most 100 mounted activity rows", async () => {
  const user = userEvent.setup();
  const props = turnPanelProps(4146);
  render(<ConversationActivityPanel {...props} conversation={{ ...props.conversation, messages: [] }} />);
  const turn = await screen.findByRole("button", { name: "Developer · Turn 1" });
  expect(within(turn).getByText("4146 tool calls")).toBeVisible();
  await user.click(turn);
  const records = screen.getByRole("list", { name: "This turn · Execution order" });
  expect(within(records).getAllByRole("listitem")).toHaveLength(100);
  expect(screen.getByText("Page 1 of 42")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Expand page" }));
  expect(within(records).getByText("dev-turn-output-99")).toBeVisible();
  expect(within(records).queryByText("dev-turn-output-100")).not.toBeInTheDocument();
  scrollTowardEnd(records, 300);
  expect(screen.getByText("Page 1 of 42")).toBeVisible();
  expect(within(records).getByText("dev-turn-output-99")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Next page" }));
  expect(screen.getByText("Page 2 of 42")).toBeVisible();
  expect(within(records).getByRole("button", { name: /dev-turn-command-100$/ })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  expect(within(records).getAllByRole("listitem")).toHaveLength(100);
  scrollTowardEnd(records, 300);
  await user.click(screen.getByRole("button", { name: "Previous page" }));
  expect(screen.getByText("Page 1 of 42")).toBeVisible();
});

it("loads older history at the scroll boundary, then merges live messages and catches up", async () => {
  const { conversation, ...props } = turnPanelProps();
  const message = (id: string, content: string) => ({ id, content, sender_id: "dev" });
  const recent = message("recent", "Recent activity");
  const live = message("live", "Live activity");
  const gap = message("gap", "Missed activity");
  const older = message("older", "Earlier activity");
  const afterGap = message("after-gap", "Another missed activity");
  type Page = Awaited<ReturnType<typeof fetchMessagePageRequest>>;
  let resolveEarlier!: (page: Page) => void;
  const earlierPage = new Promise<Page>((resolve) => {
    resolveEarlier = resolve;
  });
  fetch
    .mockResolvedValueOnce({ messages: [recent], has_more: true })
    .mockReturnValueOnce(earlierPage)
    .mockResolvedValueOnce({ messages: [recent, gap, afterGap, live], has_more: true });
  const subscribe = vi.mocked(subscribeIMEvents).mockClear();
  render(<ConversationActivityPanel {...props} conversation={{ ...conversation, messages: [], is_direct: true }} />);
  expect(await screen.findByText("Recent activity")).toBeVisible();
  const list = screen.getByRole("list");
  scrollTowardEnd(list, 250);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: "Load earlier activity" })).toBeVisible();
  scrollTowardEnd(list, 300);
  scrollTowardEnd(list, 300);
  expect(screen.getByRole("status")).toHaveTextContent("Loading activity");
  expect(fetch).toHaveBeenCalledTimes(2);
  await act(async () => resolveEarlier({ messages: [older], has_more: false }));
  expect(await screen.findByText("Earlier activity")).toBeVisible();
  expect(screen.queryByRole("button", { name: "Load earlier activity" })).not.toBeInTheDocument();
  scrollTowardEnd(list, 300);
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(fetch).toHaveBeenLastCalledWith("room", expect.objectContaining({ before: recent.id, limit: 100 }));

  const emit = subscribe.mock.calls[0][0];
  act(() => {
    emit({ type: "message.created", room_id: "room", message: live });
    emit({ type: "message.created", room_id: "room", message: live });
  });
  expect(await screen.findByText("Live activity")).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(2);

  act(() => emit({ type: "connection.open" }));
  expect(await screen.findByText("Missed activity")).toBeVisible();
  expect(screen.getByText("Another missed activity")).toBeVisible();
  expect(screen.getAllByText("Live activity")).toHaveLength(1);
  expect(screen.getByText("Earlier activity")).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(3);
});

it("keeps the newest SSE revision when a refresh returns an older copy of the same turn", async () => {
  const user = userEvent.setup();
  const props = turnPanelProps(2);
  const [source, dev] = props.conversation.messages;
  if (!("metadata" in dev)) throw new Error("Expected a turn progress fixture");
  const progress = dev.metadata.csgclaw.turn_progress;
  const revised = {
    ...dev,
    metadata: {
      ...dev.metadata,
      csgclaw: { turn_progress: { ...progress, revision: 2, items: progress.items.slice(0, 1) } },
    },
  };
  const subscribe = vi.mocked(subscribeIMEvents).mockClear();
  type Page = Awaited<ReturnType<typeof fetchMessagePageRequest>>;
  let resolve!: (page: Page) => void;
  const promise = new Promise<Page>((complete) => {
    resolve = complete;
  });
  fetch.mockResolvedValueOnce({ messages: [source, dev], has_more: false }).mockReturnValueOnce(promise);
  render(<ConversationActivityPanel {...props} />);
  expect(await screen.findByText("2 tool calls")).toBeVisible();
  const refresh = screen.getByRole("button", { name: "Refresh activity" });
  await waitFor(() => expect(refresh).toBeEnabled());
  await user.click(refresh);
  await act(async () => {
    subscribe.mock.calls[0][0]({ type: "message.created", room_id: "room", message: revised });
    resolve({ messages: [source, dev], has_more: false });
  });
  expect(await screen.findByText("1 tool calls")).toBeVisible();
  expect(screen.queryByText("2 tool calls")).not.toBeInTheDocument();
});

it("recovers moved final replies and out-of-window running turns in one refresh", async () => {
  const { conversation } = turnPanelProps(1);
  const [, dev, tester] = conversation.messages;
  if (!("metadata" in dev) || !("metadata" in tester)) throw new Error("Expected progress fixtures");
  const snapshot = (message: typeof dev, revision: number, status: string) => ({
    ...message,
    metadata: {
      ...message.metadata,
      csgclaw: { turn_progress: { ...message.metadata.csgclaw.turn_progress, revision, status } },
    },
  });
  const active = snapshot(dev, 1, "running");
  const moving = snapshot(tester, 1, "waiting");
  const finished = snapshot(dev, 2, "failed");
  const final = { ...snapshot(tester, 2, "succeeded"), created_at: "2026-11-13T01:00:00Z" };
  const newer = Array.from({ length: 99 }, (_, i) => ({ id: `new-${i}`, sender_id: "dev", content: "new message" }));
  fetch
    .mockResolvedValueOnce({ messages: [active, moving], has_more: true })
    .mockResolvedValueOnce({ messages: [...newer, final], refreshed: [finished], has_more: true });
  const { result } = renderHook(() => useActivityHistory("room", true, "[]"));
  await waitFor(() => expect(result.current.loaded).toBe(true));
  act(() => result.current.refresh());
  await waitFor(() => expect(result.current.messages).toContainEqual(finished));
  expect(result.current.messages).toContainEqual(newer[0]);
  expect(result.current.messages).toContainEqual(final);
  expect(result.current.messages).toHaveLength(101);
  expect(result.current.before).toBe("new-0");
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(fetch).toHaveBeenLastCalledWith(
    "room",
    expect.objectContaining({ limit: 100, refreshIDs: [moving.id, active.id] }),
  );
});

it("drops stale link targets after clear and opens a new link outside the recent window", async () => {
  const { conversation } = turnPanelProps();
  const [oldTarget, newTarget] = conversation.messages;
  fetch
    .mockResolvedValueOnce({ messages: [oldTarget], has_more: false })
    .mockResolvedValueOnce({ messages: [], has_more: false })
    .mockResolvedValueOnce({
      messages: [{ id: "recent", content: "new message" }],
      refreshed: [newTarget],
      has_more: true,
    });
  const subscribe = vi.mocked(subscribeIMEvents).mockClear();
  const { result, rerender } = renderHook(({ target }) => useActivityHistory("room", true, target), {
    initialProps: { target: JSON.stringify([oldTarget.id]) },
  });
  await waitFor(() => expect(result.current.messages).toContainEqual(oldTarget));
  act(() => subscribe.mock.calls[0][0]({ type: "room.messages_cleared", room_id: "room" }));
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.messages).toEqual([]);
  rerender({ target: JSON.stringify([newTarget.id]) });
  await waitFor(() => expect(result.current.messages).toContainEqual(newTarget));
  expect(fetch).toHaveBeenLastCalledWith("room", expect.objectContaining({ refreshIDs: [newTarget.id] }));
});

it("loads one chronological history page from an upward gesture and preserves the current rows", async () => {
  const user = userEvent.setup();
  const { conversation, ...props } = turnPanelProps();
  const recent = { id: "recent", sender_id: "dev", content: "Recent activity", created_at: "2026-11-12T00:01:00Z" };
  const older = { id: "older", sender_id: "dev", content: "Earlier activity", created_at: "2026-11-12T00:00:00Z" };
  fetch
    .mockResolvedValueOnce({ messages: [recent], has_more: true })
    .mockResolvedValueOnce({ messages: [older], has_more: true });
  render(<ConversationActivityPanel {...props} conversation={{ ...conversation, messages: [], is_direct: true }} />);
  await screen.findByText("Recent activity");
  await user.click(screen.getByRole("radio", { name: "Chronological" }));
  expect(fetch).toHaveBeenCalledTimes(1);
  fireEvent.wheel(screen.getByRole("list"), { deltaY: -80 });
  expect(await screen.findByText("Page 2 of 2")).toBeVisible();
  expect(screen.getByText("Recent activity")).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(2);
  await user.click(screen.getByRole("button", { name: "Previous page" }));
  expect(screen.getByText("Earlier activity")).toBeVisible();
  expect(fetch).toHaveBeenCalledTimes(2);
});

it("waits for another gesture when a short filtered list still has earlier history", async () => {
  const user = userEvent.setup();
  const { conversation, ...props } = turnPanelProps();
  const recent = { id: "recent", sender_id: "dev", content: "Recent activity" };
  fetch
    .mockResolvedValueOnce({ messages: [recent], has_more: true })
    .mockResolvedValueOnce({ messages: [{ id: "older", sender_id: "test", content: "Filtered out" }], has_more: true });
  render(<ConversationActivityPanel {...props} conversation={{ ...conversation, messages: [] }} />);
  await screen.findByText("Recent activity");
  await user.click(screen.getByRole("button", { name: "Developer" }));
  const list = screen.getByRole("list");
  fireEvent.wheel(list, { deltaY: 80 });
  await waitFor(() => expect(screen.getByRole("button", { name: "Load earlier activity" })).toBeEnabled());
  fireEvent.wheel(list, { deltaY: 80 });
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(screen.queryByText("Filtered out")).not.toBeInTheDocument();
});

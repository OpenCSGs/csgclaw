import { useRef, useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createQueryWrapper } from "../../../../tests/helpers/queryClient";
import { ConversationResourcePicker } from "./ConversationResourcePicker";
import { useComposerResourcePicker } from "./useComposerResourcePicker";
import { ComposerAddMenu } from "@/components/business/ConversationPane/ComposerAddMenu";
import { ResourceTag } from "@/components/business/ResourceTag";
import { fetchConversationResourceBatch } from "@/api/agentResources";
import {
  resourceKey,
  maxResourceReferences,
  type AgentResourceSummary,
  type ResourceSnapshot,
} from "@/models/conversationResources";

vi.mock("@/api/agentResources", () => ({ fetchConversationResourceBatch: vi.fn() }));
const t = (key: string) => key;
const item = (id: string, available = true): AgentResourceSummary => ({
  source: "app",
  id,
  name: "GitLab",
  type: "connector",
  enabled: available,
  available,
});

it("fits the popup above its actual anchor within desktop and clipping boundaries", async () => {
  vi.mocked(fetchConversationResourceBatch).mockResolvedValue({
    items: [item("first")],
    total: 1,
    list_revision: "revision",
    has_more: false,
  });
  let bottom = 280;
  const original = HTMLElement.prototype.getBoundingClientRect;
  const bounds = vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (
    this: HTMLElement,
  ) {
    if (this.tagName === "SECTION" && this.getAttribute("aria-label") === "composerResourceConnector") {
      return { ...original.call(this), bottom } as DOMRect;
    }
    if (this.dataset.testid === "clipping-container") {
      return { ...original.call(this), top: 100 } as DOMRect;
    }
    return original.call(this);
  });
  document.documentElement.style.setProperty("--app-overlay-top-inset", "48px");
  try {
    const { wrapper } = createQueryWrapper();
    render(
      <div data-testid="clipping-container" style={{ overflowY: "hidden" }}>
        <ConversationResourcePicker agentID="agent" kind="connector" t={t} onClose={() => {}} onPick={() => {}} />
      </div>,
      { wrapper },
    );
    const popup = screen.getByRole("region", { name: "composerResourceConnector" });
    expect(popup).toHaveStyle({ maxHeight: "148px" });
    bottom = 220;
    fireEvent(window, new Event("resize"));
    await waitFor(() => expect(popup).toHaveStyle({ maxHeight: "88px" }));
    screen.getByTestId("clipping-container").style.overflowY = "visible";
    fireEvent(window, new Event("resize"));
    await waitFor(() => expect(popup).toHaveStyle({ maxHeight: "140px" }));
    document.documentElement.style.removeProperty("--app-overlay-top-inset");
    fireEvent(window, new Event("resize"));
    await waitFor(() => expect(popup).toHaveStyle({ maxHeight: "212px" }));
  } finally {
    bounds.mockRestore();
    document.documentElement.style.removeProperty("--app-overlay-top-inset");
  }
});

function Composer({ agentID = "agent", disabled = false }: { agentID?: string; disabled?: boolean }) {
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const [draft, setDraft] = useState("Existing draft");
  const [selected, setSelected] = useState<ResourceSnapshot[]>([]);
  const picker = useComposerResourcePicker({
    editorRef,
    resourceAgentID: agentID,
    disabled,
    selected,
    onPickResources: (items) => setSelected((current) => [...current, ...items]),
  });
  return (
    <>
      {selected.map((resource) => (
        <ResourceTag key={resourceKey(resource)} resource={resource} />
      ))}
      {picker.pickerProps ? (
        <ConversationResourcePicker key={`${agentID}:${picker.pickerProps.kind}`} {...picker.pickerProps} t={t} />
      ) : null}
      <textarea
        ref={editorRef}
        aria-label="Draft"
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        hidden={picker.active}
      />
      <ComposerAddMenu disabled={disabled} t={t} onOpenResourcePicker={picker.open} onAddFiles={() => {}} />
      <button>Outside</button>
    </>
  );
}

it("searches and paginates on the server while preserving resource identities and availability", async () => {
  const fetch = vi.mocked(fetchConversationResourceBatch);
  fetch.mockImplementation(async (_agent, _type, cursor, request) => ({
    items:
      request?.search === "special"
        ? [item("searched")]
        : cursor
          ? [item("second")]
          : [item("first"), item("disabled", false)],
    total: 3,
    list_revision: "revision",
    has_more: !cursor && !request?.search,
    next_cursor: !cursor && !request?.search ? "page-2" : undefined,
  }));
  const pick = vi.fn();
  const { wrapper } = createQueryWrapper();
  render(<ConversationResourcePicker agentID="agent" kind="connector" t={t} onClose={() => {}} onPick={pick} />, {
    wrapper,
  });
  const user = userEvent.setup();
  await screen.findByTitle("app:first");
  expect(screen.getByTitle("app:disabled")).toBeDisabled();
  await user.click(screen.getByTitle("app:first"));
  expect(pick).toHaveBeenLastCalledWith(expect.objectContaining({ source: "app", id: "first" }));
  await user.click(screen.getByRole("button", { name: "taskEntryLoadMore" }));
  await user.click(await screen.findByTitle("app:second"));
  expect(pick).toHaveBeenLastCalledWith(expect.objectContaining({ source: "app", id: "second" }));
  await user.type(screen.getByRole("combobox"), "special");
  await screen.findByTitle("app:searched");
  expect(screen.getAllByRole("option")).toHaveLength(1);
  await waitFor(() =>
    expect(fetch).toHaveBeenCalledWith("agent", "connector", "", { search: "special" }, expect.any(AbortSignal)),
  );
  expect(screen.getByRole("link", { name: "composerResourceManage" })).toHaveAttribute(
    "href",
    "#/agents/agent?tab=connectors",
  );
});

it("selects directly from composer search, preserves the draft and ignores IME confirmation", async () => {
  vi.mocked(fetchConversationResourceBatch).mockResolvedValue({
    items: [item("first"), item("disabled", false), item("second")],
    total: 3,
    list_revision: "r",
    has_more: false,
  });
  const { wrapper } = createQueryWrapper();
  render(<Composer />, { wrapper });
  const user = userEvent.setup();
  const trigger = screen.getByRole("button", { name: "composerAddContent" });
  async function openPicker() {
    await user.click(trigger);
    await user.click(screen.getByRole("button", { name: "composerResourceConnector" }));
    await screen.findByTitle("app:first");
    expect(screen.getByRole("combobox")).toHaveFocus();
    expect(screen.queryByRole("textbox", { name: "Draft" })).not.toBeInTheDocument();
  }
  await openPicker();
  fireEvent.keyDown(screen.getByRole("combobox"), { key: "Enter", isComposing: true });
  fireEvent.keyDown(screen.getByRole("combobox"), { key: "Escape", isComposing: true });
  expect(screen.getByRole("listbox")).toBeInTheDocument();
  await user.keyboard("{ArrowDown}{Enter}");
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  expect(screen.getByTitle("app:second")).toHaveClass("resource-tag");
  const draft = screen.getByRole("textbox", { name: "Draft" });
  await waitFor(() => expect(draft).toHaveFocus());
  expect(draft).toHaveValue("Existing draft");
  await user.keyboard(" continued");
  await openPicker();
  expect(screen.getByRole("option", { name: "GitLab", selected: true })).toHaveAttribute("title", "app:first");
  expect(screen.getByRole("listbox").querySelector('[title="app:second"]')).toBeDisabled();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(draft).toHaveFocus());
  expect(draft).toHaveValue("Existing draft continued");
  await openPicker();
  await user.click(screen.getByRole("button", { name: "Outside" }));
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Outside" })).toHaveFocus();
});

it("resets search when changing agents or disabling the composer", async () => {
  vi.mocked(fetchConversationResourceBatch).mockResolvedValue({
    items: [item("first")],
    total: 1,
    list_revision: "r",
    has_more: false,
  });
  const { wrapper } = createQueryWrapper();
  const { rerender } = render(<Composer />, { wrapper });
  const user = userEvent.setup();
  async function openPicker() {
    await user.click(screen.getByRole("button", { name: "composerAddContent" }));
    await user.click(screen.getByRole("button", { name: "composerResourceConnector" }));
    await screen.findByRole("option");
  }
  await openPicker();
  await user.type(screen.getByRole("combobox"), "old search");
  rerender(<Composer agentID="other" />);
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  await openPicker();
  expect(screen.getByRole("combobox")).toHaveValue("");
  rerender(<Composer agentID="other" disabled />);
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  rerender(<Composer agentID="other" />);
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
});

it("prevents selecting resources beyond the reference limit", async () => {
  vi.mocked(fetchConversationResourceBatch).mockResolvedValue({
    items: [item("new")],
    total: 1,
    list_revision: "r",
    has_more: false,
  });
  const pick = vi.fn();
  const { wrapper } = createQueryWrapper();
  render(
    <ConversationResourcePicker
      agentID="agent"
      kind="connector"
      t={t}
      onClose={() => {}}
      onPick={pick}
      existing={Array.from({ length: maxResourceReferences }, (_, index) => item(String(index)))}
    />,
    { wrapper },
  );
  expect(await screen.findByRole("option")).toBeDisabled();
  await userEvent.setup().keyboard("{Enter}");
  expect(pick).not.toHaveBeenCalled();
});

it("keeps the picker open through pagination and dismisses on a non-focusable outside target", async () => {
  let finishPage!: (page: Awaited<ReturnType<typeof fetchConversationResourceBatch>>) => void;
  vi.mocked(fetchConversationResourceBatch).mockImplementation(async (_agent, _type, cursor) =>
    cursor
      ? new Promise((resolve) => {
          finishPage = resolve;
        })
      : {
          items: [item("first")],
          total: 2,
          list_revision: "r",
          has_more: true,
          next_cursor: "next",
        },
  );
  const { wrapper } = createQueryWrapper();
  render(
    <>
      <Composer />
      <div data-testid="outside-surface">Outside surface</div>
    </>,
    { wrapper },
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "composerAddContent" }));
  await user.click(screen.getByRole("button", { name: "composerResourceSkill" }));
  await screen.findByTitle("app:first");
  await user.click(screen.getByRole("button", { name: "taskEntryLoadMore" }));
  expect(screen.getByRole("combobox")).toHaveFocus();
  expect(screen.getByRole("button", { name: "loading" })).toBeDisabled();
  fireEvent.blur(screen.getByRole("combobox"), { relatedTarget: null });
  expect(screen.getByRole("listbox")).toBeInTheDocument();
  finishPage({ items: [item("second")], total: 2, list_revision: "r", has_more: false });
  await screen.findByTitle("app:second");
  expect(screen.getByTitle("app:first")).toBeInTheDocument();
  await user.click(screen.getByTestId("outside-surface"));
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
});

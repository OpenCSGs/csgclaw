import { useRef } from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useFileMentions } from "@/components/business/ConversationPane/FileMentionPicker/useFileMentions";
import { ChatSidePanel, useChatTools } from "@/components/business/ChatSidePanel";
import { createTranslator } from "@/shared/i18n";
import { parseComposerSegments } from "@/models/composer";
import {
  fetchRoomFileMessages,
  fetchRoomFiles,
  fetchWorkspaceDirectory,
  fetchWorkspaceLocation,
  openWorkspaceLocation,
} from "@/api/chatFiles";
import type { IMConversation } from "@/models/conversations";
import { createQueryWrapper } from "../helpers/queryClient";

vi.mock("@/api/chatFiles", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/chatFiles")>()),
  fetchRoomFiles: vi.fn(),
  fetchRoomFileMessages: vi.fn(),
  fetchWorkspaceDirectory: vi.fn(),
  fetchWorkspaceLocation: vi.fn(),
  openWorkspaceLocation: vi.fn(),
}));
vi.mock("@/shared/realtime/imEvents", () => ({ subscribeIMEvents: vi.fn(() => () => {}) }));
const t = createTranslator("en");
const agents = [{ id: "alice", name: "Alice", role: "worker" }];
const usersById = new Map();
const conversation: IMConversation = {
  id: "sidebar-test",
  title: "Alice",
  is_direct: true,
  members: ["admin", "alice"],
  messages: [],
};

function Harness({
  room = conversation,
  availableAgents = agents,
}: {
  room?: IMConversation;
  availableAgents?: typeof agents;
}) {
  const editorRef = useRef<HTMLDivElement>(null);
  const tools = useChatTools({
    conversation: room,
    currentUserID: "admin",
    agents: availableAgents,
    usersById,
    editorRef,
    t,
  });
  return (
    <main className="chat-panel">
      <header className="chat-header">{tools.toolbar}</header>
      <div className="messages" />
      <footer className="composer">
        <div ref={editorRef} role="textbox" contentEditable suppressContentEditableWarning />
      </footer>
      <ChatSidePanel
        controller={tools.controller}
        fileScope={tools.fileScope}
        conversation={room}
        agents={agents}
        usersById={usersById}
        locale="en"
        t={t}
        onReference={tools.onReference}
        onPreview={(item) => tools.openPreview({ items: [item], index: 0 }, "files")}
        onSources={tools.openSources}
        onCollapse={tools.collapse}
        onThreadSelect={() => {}}
        onThreadClose={() => {}}
      />
    </main>
  );
}

function MentionHarness() {
  const editorRef = useRef<HTMLDivElement>(null);
  const mentions = useFileMentions({
    scope: { roomID: "directory-mention-test", userID: "admin", agents, defaultAgentID: "alice" },
    editorRef,
    onSync: () => {},
    people: [],
    onPerson: () => {},
    t,
  });
  return (
    <>
      <div
        ref={editorRef}
        role="textbox"
        contentEditable
        suppressContentEditableWarning
        onInput={mentions.sync}
        onKeyDown={mentions.handleKeyDown}
      />
      {mentions.picker}
    </>
  );
}

describe("shared chat sidebar", () => {
  it("hides Agent selection in a direct chat, displays the workspace breadcrumb and opens its path through the menu", async () => {
    const user = userEvent.setup();
    render(<Harness />, { wrapper: createQueryWrapper().wrapper });
    await user.click(screen.getByRole("button", { name: "Files" }));
    expect(
      within(await screen.findByRole("navigation", { name: "Workspace files" })).getByText("Alice"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Workspace agent" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Open file or directory" }));
    await user.click(screen.getByRole("menuitem", { name: "Open in file manager" }));
    await waitFor(() => expect(openWorkspaceLocation).toHaveBeenCalledWith("alice", "sidebar-test", "", "system"));
    await user.click(screen.getByRole("button", { name: /^README.md/ }));
    await user.click(screen.getByRole("button", { name: "Open file or directory" }));
    await user.click(await screen.findByRole("menuitem", { name: "Visual Studio Code" }));
    await waitFor(() =>
      expect(openWorkspaceLocation).toHaveBeenCalledWith("alice", "sidebar-test", "README.md", "vscode"),
    );
  });

  it("uses the workspace root name as the first breadcrumb and returns to it from a directory", async () => {
    const user = userEvent.setup();
    let releaseNested = () => {};
    const nestedGate = new Promise<void>((resolve) => {
      releaseNested = resolve;
    });
    vi.mocked(fetchWorkspaceDirectory).mockImplementation(async (_agent, path) => {
      if (path) await nestedGate;
      return {
        absolute_path: `/workspace/Alice${path ? `/${path}` : ""}`,
        open_targets: [],
        entries: path
          ? [{ path: "docs/guide.md", name: "guide.md", type: "file" }]
          : [{ path: "docs", name: "docs", type: "dir" }],
      };
    });
    render(<Harness />, { wrapper: createQueryWrapper().wrapper });
    await user.click(screen.getByRole("button", { name: "Files" }));
    const breadcrumb = await screen.findByRole("navigation", { name: "Workspace files" });
    expect(within(breadcrumb).getByText("Alice")).toHaveAttribute("aria-current", "page");
    expect(breadcrumb.querySelector("img")).not.toBeInTheDocument();

    await user.click(await screen.findByRole("button", { name: "docs" }));
    expect(within(breadcrumb).getByRole("button", { name: "Alice" })).toBeInTheDocument();
    expect(within(breadcrumb).getByText("docs")).toHaveAttribute("aria-current", "page");
    releaseNested();
    await screen.findByRole("button", { name: /^guide.md/ });
    await user.click(within(breadcrumb).getByRole("button", { name: "Alice" }));
    expect(within(breadcrumb).getByText("Alice")).toHaveAttribute("aria-current", "page");
  });

  it("keeps Agent selection available in a room", async () => {
    const user = userEvent.setup();
    render(<Harness room={{ ...conversation, is_direct: false }} />, { wrapper: createQueryWrapper().wrapper });
    await user.click(screen.getByRole("button", { name: "Files" }));
    expect(await screen.findByRole("combobox", { name: "Workspace agent" })).toBeInTheDocument();
  });

  it("defaults a room to its Manager workspace even when workers appear first", async () => {
    const user = userEvent.setup();
    render(
      <Harness
        room={{
          ...conversation,
          id: "room-manager-default",
          is_direct: false,
          members: ["admin", "alice", "agent-manager"],
        }}
        availableAgents={[...agents, { id: "agent-manager", name: "Manager", role: "manager" }]}
      />,
      { wrapper: createQueryWrapper().wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Files" }));
    expect(await screen.findByRole("combobox", { name: "Workspace agent" })).toHaveTextContent("Manager");
  });

  it("references a directory while preserving the existing draft", async () => {
    const user = userEvent.setup();
    render(<Harness />, { wrapper: createQueryWrapper().wrapper });
    const editor = screen.getByRole("textbox");
    editor.textContent = "Existing draft ";
    editor.focus();
    const range = document.createRange();
    range.selectNodeContents(editor);
    range.collapse(false);
    window.getSelection()?.removeAllRanges();
    window.getSelection()?.addRange(range);
    document.dispatchEvent(new Event("selectionchange"));
    await user.click(screen.getByRole("button", { name: "Files" }));
    await user.click(await screen.findByRole("button", { name: "Reference file docs" }));
    await waitFor(() => expect(editor.textContent).toContain("Existing draft @docs/"));
    expect(parseComposerSegments(editor).filter((segment) => segment.type === "file")).toEqual([
      expect.objectContaining({
        reference: expect.objectContaining({ source: "workspace", agent_id: "alice", path: "docs" }),
      }),
    ]);
  });

  it("references directories with Enter, browses with Tab, and preserves IME composition", async () => {
    const user = userEvent.setup();
    render(<MentionHarness />, { wrapper: createQueryWrapper().wrapper });
    const editor = screen.getByRole("textbox");
    await user.click(editor);
    await user.keyboard("@docs");
    await screen.findByRole("option", { name: /@docs\// });
    fireEvent.keyDown(editor, { key: "Enter", isComposing: true });
    expect(editor.textContent).toBe("@docs");
    await user.keyboard("{Tab}");
    expect(editor.textContent).toBe("@docs/");
    editor.textContent = "";
    await user.click(editor);
    await user.keyboard("@docs");
    await screen.findByRole("option", { name: /@docs\// });
    await user.keyboard("{Enter}");
    expect(parseComposerSegments(editor).filter((segment) => segment.type === "file")).toEqual([
      expect.objectContaining({ reference: expect.objectContaining({ source: "workspace", path: "docs" }) }),
    ]);
  });

  it("opens a new tab from plus and lets the user choose a tool inside it", async () => {
    const user = userEvent.setup();
    const wrapper = createQueryWrapper().wrapper;
    const view = render(<Harness />, { wrapper });
    await user.click(screen.getByRole("button", { name: "Files" }));
    const panel = screen.getByRole("complementary");
    await user.click(within(panel).getByRole("button", { name: "New tab" }));
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(within(panel).getByRole("tab", { name: "New tab" })).toHaveAttribute("aria-selected", "true");
    view.unmount();
    render(<Harness />, { wrapper });
    expect(screen.getByRole("tab", { name: "New tab" })).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("button", { name: /^Browser.*Enter/ }));
    expect(screen.queryByRole("tab", { name: "New tab" })).not.toBeInTheDocument();
    expect(screen.getAllByRole("tab")).toHaveLength(2);
    const address = screen.getByRole("textbox", { name: "Web address" });
    await user.type(address, "example.org");
    expect(screen.getByRole("link", { name: "Open in system browser" })).toHaveAttribute(
      "href",
      "https://example.org/",
    );
    await user.keyboard("{Enter}");
    expect(screen.getByTitle("Browser")).toHaveAttribute("src", "https://example.org/");
    expect(screen.getByRole("tab", { name: "example.org" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getAllByRole("link")).toHaveLength(2);
  });

  it("places close all beside the new tab action and returns to the tool picker", async () => {
    const user = userEvent.setup();
    render(<Harness />, { wrapper: createQueryWrapper().wrapper });
    await user.click(screen.getByRole("button", { name: "Files" }));
    const panel = screen.getByRole("complementary");
    const actions = panel.querySelector(".chat-side-leading-actions");
    expect(actions?.nextElementSibling).toHaveClass("chat-side-tabs");
    expect(actions?.querySelectorAll("button")[0]).toHaveAttribute("aria-label", "New tab");
    expect(actions?.querySelectorAll("button")[1]).toHaveAttribute("aria-label", "Close all tabs");
    await user.click(within(panel).getByRole("button", { name: "Close all tabs" }));
    expect(within(panel).queryAllByRole("tab")).toHaveLength(0);
    expect(within(panel).getByRole("button", { name: /Workspace files/ })).toBeVisible();
  });

  beforeEach(() => {
    localStorage.clear();
    vi.mocked(fetchWorkspaceLocation).mockResolvedValue({
      absolute_path: "/workspace/Alice/README.md",
      directory: false,
      open_targets: ["system", "vscode"],
    });
    vi.mocked(openWorkspaceLocation).mockResolvedValue(undefined);
    vi.mocked(fetchWorkspaceDirectory).mockResolvedValue({
      absolute_path: "/workspace/Alice",
      open_targets: ["system", "terminal"],
      entries: [
        { path: "docs", name: "docs", type: "dir" },
        { path: "README.md", name: "README.md", type: "file", size: 10 },
      ],
    });
    vi.mocked(fetchRoomFileMessages).mockResolvedValue([
      {
        id: "message",
        sender_id: "admin",
        content: "",
        created_at: "2026-10-14T00:00:00Z",
        attachments: [
          {
            id: "attachment-id",
            name: "report.pdf",
            media_type: "application/pdf",
            size_bytes: 30,
            kind: "file",
            created_at: "2026-10-14T00:00:00Z",
            sha256: "test",
            download_url: "/api/v1/attachments/attachment-id?token=test-capability",
          },
        ],
      },
    ]);
    vi.mocked(fetchRoomFiles).mockResolvedValue({
      items: [
        {
          id: "attachment-id",
          name: "report.pdf",
          media_type: "application/pdf",
          size_bytes: 30,
          message_id: "message",
        },
      ],
      from: 0,
      limit: 50,
      total: 1,
    });
  });
  it("retains tabs on collapse, restores per-conversation state, and does not persist capabilities", async () => {
    const user = userEvent.setup();
    const wrapper = createQueryWrapper().wrapper;
    const view = render(<Harness />, { wrapper });
    expect(screen.getByRole("complementary", { hidden: true })).not.toBeVisible();
    await user.click(screen.getByRole("button", { name: "Files" }));
    await screen.findByRole("button", { name: /^README.md/ });
    const panel = screen.getByRole("complementary");
    expect(within(panel).getByRole("tab", { name: "Files" }).querySelector(".chat-side-folder-icon")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
    expect(within(panel).getByRole("searchbox", { name: "Search files" })).toHaveAttribute("data-size", "sm");
    await user.click(within(panel).getByRole("tab", { name: "Conversation attachments" }));
    await screen.findByRole("button", { name: /^report.pdf/ });
    await user.click(within(panel).getByRole("button", { name: "Collapse sidebar" }));
    expect(panel).not.toBeVisible();
    await user.click(screen.getByRole("button", { name: "Files" }));
    expect(within(panel).getByRole("tab", { name: "Files" })).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("button", { name: "Browser" }));
    const address = screen.getByRole("textbox", { name: "Web address" });
    await user.type(address, "javascript:alert(1)");
    await user.keyboard("{Enter}");
    expect(screen.getByRole("alert")).toHaveTextContent("Enter a valid HTTP or HTTPS URL");
    await user.clear(address);
    await user.type(address, "https://example.org/");
    await user.keyboard("{Enter}");
    expect(screen.getByTitle("Browser")).toHaveAttribute("sandbox", "allow-scripts allow-forms");
    const stored = Object.keys(localStorage)
      .filter((key) => key.includes("chatSidePanel.v1"))
      .map((key) => localStorage.getItem(key))
      .join("");
    expect(stored).not.toContain("downloadURL");
    view.unmount();
    render(<Harness />, { wrapper });
    expect(screen.getByRole("tab", { name: "example.org" })).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("button", { name: "Close tab example.org" }));
    expect(screen.getByRole("tab", { name: "Files" })).toHaveAttribute("aria-selected", "true");
  });
  it("updates conversation attachments when a message adds a file without refetching for text-only messages", async () => {
    const user = userEvent.setup();
    const initialMessage = {
      id: "message",
      sender_id: "admin",
      content: "",
      created_at: "2026-10-14T00:00:00Z",
      attachments: [
        {
          id: "attachment-id",
          name: "report.pdf",
          media_type: "application/pdf",
          size_bytes: 30,
          kind: "file",
          created_at: "2026-10-14T00:00:00Z",
          sha256: "test",
          download_url: "/api/v1/attachments/attachment-id?token=test-capability",
        },
      ],
    };
    const textMessage = {
      id: "text-message",
      sender_id: "admin",
      content: "Hello",
      created_at: "2026-10-14T00:01:00Z",
      attachments: [],
    };
    const uploadedMessage = {
      id: "upload-message",
      sender_id: "admin",
      content: "",
      created_at: "2026-10-14T00:02:00Z",
      attachments: [
        {
          id: "new-attachment-id",
          name: "new-image.svg",
          media_type: "image/svg+xml",
          size_bytes: 42,
          kind: "file",
          created_at: "2026-10-14T00:02:00Z",
          sha256: "new-test",
          download_url: "/api/v1/attachments/new-attachment-id?token=new-capability",
        },
      ],
    };
    vi.mocked(fetchRoomFileMessages).mockResolvedValue([initialMessage]);
    const view = render(<Harness room={{ ...conversation, messages: [initialMessage] }} />, {
      wrapper: createQueryWrapper().wrapper,
    });
    await user.click(screen.getByRole("button", { name: "Files" }));
    await user.click(screen.getByRole("tab", { name: "Conversation attachments" }));
    await screen.findByRole("button", { name: /^report.pdf/ });
    const initialFetches = vi.mocked(fetchRoomFileMessages).mock.calls.length;

    view.rerender(<Harness room={{ ...conversation, messages: [initialMessage, textMessage] }} />);
    expect(fetchRoomFileMessages).toHaveBeenCalledTimes(initialFetches);

    vi.mocked(fetchRoomFileMessages).mockResolvedValue([initialMessage, textMessage, uploadedMessage]);
    view.rerender(<Harness room={{ ...conversation, messages: [initialMessage, textMessage, uploadedMessage] }} />);
    await screen.findByRole("button", { name: /^new-image.svg/ });
    expect(vi.mocked(fetchRoomFileMessages).mock.calls.length).toBeGreaterThan(initialFetches);
  });
  it("returns to the file directory after closing a file preview beside a browser tab", async () => {
    const user = userEvent.setup();
    render(<Harness />, { wrapper: createQueryWrapper().wrapper });
    await user.click(screen.getByRole("button", { name: "Files" }));
    await screen.findByRole("button", { name: /^README.md/ });
    await user.click(screen.getByRole("button", { name: "Browser" }));
    await user.click(screen.getByRole("tab", { name: "Files" }));
    await user.click(screen.getByRole("button", { name: /^README.md/ }));
    expect(screen.getByRole("tab", { name: "README.md" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "README.md" }).querySelector(".chat-side-file-icon")).toBeInTheDocument();
    const closePreview = screen.getByRole("button", { name: "Close tab README.md" });
    expect(closePreview).toHaveAttribute("data-csg-control", "button");
    await user.click(closePreview);
    expect(screen.getByRole("tab", { name: "Files" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("button", { name: /^README.md/ })).toBeVisible();
  });
  it("inserts a source-bound file reference at the saved caret without losing surrounding text", async () => {
    const user = userEvent.setup();
    render(<Harness />, { wrapper: createQueryWrapper().wrapper });
    const editor = screen.getByRole("textbox");
    editor.textContent = "before after";
    editor.focus();
    const range = document.createRange();
    range.setStart(editor.firstChild!, 7);
    range.collapse(true);
    window.getSelection()?.removeAllRanges();
    window.getSelection()?.addRange(range);
    act(() => document.dispatchEvent(new Event("selectionchange")));
    await user.click(screen.getByRole("button", { name: "Files" }));
    const button = await screen.findByRole("button", { name: "Reference file README.md" });
    await user.click(button);
    await waitFor(() => expect(editor).toHaveTextContent("before @README.md after"));
    expect(parseComposerSegments(editor)).toContainEqual({
      type: "file",
      text: "@README.md",
      reference: { source: "workspace", agent_id: "alice", path: "README.md", name: "README.md", label: "README.md" },
    });
  });

  it("keeps workspace queries separate for tasks using the same Agent", async () => {
    const user = userEvent.setup();
    vi.mocked(fetchWorkspaceDirectory).mockImplementation(async (_agent, _path, _signal, roomID) => ({
      entries: [{ path: `${roomID}.md`, name: `${roomID}.md`, type: "file" }],
    }));
    const view = render(<Harness room={{ ...conversation, id: "first-task" }} />, {
      wrapper: createQueryWrapper().wrapper,
    });
    await user.click(screen.getByRole("button", { name: "Files" }));
    await screen.findByRole("button", { name: /^first-task.md/ });
    view.rerender(<Harness room={{ ...conversation, id: "second-task" }} />);
    await user.click(screen.getByRole("button", { name: "Files" }));
    await screen.findByRole("button", { name: /^second-task.md/ });
    expect(screen.queryByRole("button", { name: /^first-task.md/ })).not.toBeInTheDocument();
    expect(fetchWorkspaceDirectory).toHaveBeenCalledWith("alice", "", expect.any(AbortSignal), "second-task");
  });
  it("supports application maximum size and gives Escape precedence to restoring layout", async () => {
    const user = userEvent.setup();
    render(<Harness />, { wrapper: createQueryWrapper().wrapper });
    await user.click(screen.getByRole("button", { name: "Files" }));
    const panel = screen.getByRole("complementary");
    await user.click(within(panel).getByRole("button", { name: "Maximize sidebar" }));
    expect(panel).toHaveClass("is-maximized");
    fireEvent.keyDown(window, { key: "Escape" });
    expect(panel).not.toHaveClass("is-maximized");
    expect(panel).toBeVisible();
    fireEvent.keyDown(window, { key: "Escape" });
    expect(panel).not.toBeVisible();
  });
});

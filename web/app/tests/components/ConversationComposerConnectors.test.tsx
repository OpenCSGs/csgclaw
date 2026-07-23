import { createRef } from "react";
import { createEvent, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConversationComposer } from "@/components/business/ConversationPane/ConversationComposer";
import type { ConversationComposerProps } from "@/components/business/ConversationPane/ConversationComposer";
import { createAttachmentDrafts } from "@/models/attachments";
import { emptyGitHubConnectorStatus, emptyGitLabConnectorStatus } from "@/models/connectors";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key, params) => {
  const labels: Record<string, string> = {
    connectorCallbackURL: "Callback URL",
    connectorClientID: "Client ID",
    connectorClientSecret: "Client Secret",
    connectorConnect: "Connect",
    connectorConnected: "Connected",
    connectorDisconnect: "Disconnect",
    connectorGitHub: "GitHub",
    connectorGitLab: "GitLab",
    connectorGitLabBaseURL: "GitLab Base URL",
    connectorGitLabToken: "Personal Access Token",
    connectorGitLabTokenKeep: "Leave blank to keep the current token",
    connectorManage: "Manage",
    connectorManagerTitle: "Manage connectors",
    connectorNotConnected: "Not connected",
    connectorSave: "Save",
    connectorScopes: "Scopes",
    connectorSetUp: "Set up",
    composerAdd: "Add",
    composerAddContent: "Add content",
    composerFiles: "Files",
    composerConnectors: "Connectors",
    inputPlaceholder: "Message",
    addAttachment: "Add attachment",
    attachments: "Attachments",
    attachmentsScrollPrevious: "View previous attachments",
    attachmentsScrollNext: "View more attachments",
    removeAttachment: "Remove attachment",
    removeAttachmentNamed: `Remove attachment: ${params?.name ?? ""}`,
    composerTip: "Enter to send · Shift + Enter for a new line",
    send: "Send",
  };
  return labels[key] ?? key;
};

function renderComposer(props: Partial<ConversationComposerProps> = {}): ReturnType<typeof render> {
  return render(
    <ConversationComposer
      authBusyProvider=""
      authStatuses={{}}
      composerDisabled={false}
      composerError=""
      draftSegments={[]}
      draftText=""
      editorRef={createRef<HTMLDivElement>()}
      managerProvider=""
      mentionCandidates={[]}
      mentionIndex={0}
      mentionableUsersByName={new Map()}
      slashCandidates={[]}
      slashIndex={0}
      slashPickerLoading={false}
      slashPickerOpen={false}
      t={t}
      onApplyMention={() => {}}
      onApplySlashCandidate={() => {}}
      onComposerCompositionEnd={() => {}}
      onComposerCompositionStart={() => {}}
      onComposerKeyDown={() => {}}
      onProviderLogin={() => {}}
      onSendMessage={() => {}}
      onSyncComposer={() => {}}
      {...props}
    />,
  );
}

describe("ConversationComposer connectors", () => {
  it("blocks message entry with the provided manager runtime warning", async () => {
    const user = userEvent.setup();
    const onSendMessage = vi.fn();
    renderComposer({
      composerDisabled: true,
      composerDisabledReason: "Install Codex CLI first.",
      draftText: "hello",
      onSendMessage,
    });

    expect(screen.getByText("Install Codex CLI first.")).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Message" })).toHaveAttribute("contenteditable", "false");
    const sendButton = screen.getByRole("button", { name: "Send" });
    expect(sendButton).toBeDisabled();

    await user.click(sendButton);
    expect(onSendMessage).not.toHaveBeenCalled();
  });

  it("opens the add menu with files and connectors sections", async () => {
    const user = userEvent.setup();
    renderComposer({
      connectorStatus: {
        ...emptyGitHubConnectorStatus(),
        configured: true,
        client_id: "client-id",
        client_secret_set: true,
      },
    });

    const button = screen.getByRole("button", { name: "Add content" });
    expect(button).toHaveClass("composer-add-button");

    await user.click(button);

    const dialog = screen.getByRole("dialog", { name: "Add content" });
    expect(dialog).toBeInTheDocument();
    expect(within(dialog).getByText("Files")).toBeInTheDocument();
    expect(within(dialog).getByText("Connectors")).toBeInTheDocument();
    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("GitLab")).toBeInTheDocument();
    expect(screen.getAllByText("Not connected")).toHaveLength(2);
    expect(within(dialog).getAllByRole("button", { name: "Connect" })).toHaveLength(2);
    expect(screen.queryByLabelText("Client ID")).not.toBeInTheDocument();
    expect(screen.queryByText("Save")).not.toBeInTheDocument();
    expect(screen.queryByText("Set up")).not.toBeInTheDocument();
  });

  it("configures GitLab from the connector menu", async () => {
    const user = userEvent.setup();
    const onSaveGitLabConnectorConfig = vi.fn().mockResolvedValue(undefined);
    renderComposer({
      gitlabConnectorStatus: emptyGitLabConnectorStatus(),
      onSaveGitLabConnectorConfig,
    });

    await user.click(screen.getByRole("button", { name: "Add content" }));
    const gitlabRow = screen.getByText("GitLab").closest(".connector-provider-row") as HTMLElement;
    await user.click(within(gitlabRow).getByRole("button", { name: "Connect" }));
    await user.type(screen.getByLabelText("GitLab Base URL"), "https://gitlab.example.com/");
    await user.type(screen.getByLabelText("Personal Access Token"), "glpat-secret");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(onSaveGitLabConnectorConfig).toHaveBeenCalledWith({
      base_url: "https://gitlab.example.com/",
      access_token: "glpat-secret",
    });
    expect(screen.queryByDisplayValue("glpat-secret")).not.toBeInTheDocument();
  });

  it("places the add and send controls in one composer toolbar without visible shortcut copy", () => {
    const { container } = renderComposer();

    const row = container.querySelector(".composer-toolbar");
    expect(row).toBeInTheDocument();
    expect(within(row as HTMLElement).getByRole("button", { name: "Add content" })).toHaveClass("composer-add-button");
    expect(within(row as HTMLElement).getByRole("button", { name: "Send" })).toHaveClass("composer-send-button");
    expect(within(row as HTMLElement).getByText("Enter to send · Shift + Enter for a new line")).toHaveClass("sr-only");
    expect(container.querySelector(".composer-tip")).not.toBeInTheDocument();
  });

  it("opens the file picker from the Files menu item", async () => {
    const user = userEvent.setup();
    const inputClick = vi.spyOn(HTMLInputElement.prototype, "click");
    renderComposer();

    await user.click(screen.getByRole("button", { name: "Add content" }));
    await user.click(screen.getByRole("button", { name: "Files" }));

    expect(inputClick).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog", { name: "Add content" })).not.toBeInTheDocument();
    inputClick.mockRestore();
  });

  it("allows sending an attachment-only draft", async () => {
    const user = userEvent.setup();
    const onSendMessage = vi.fn();
    const onRemoveAttachment = vi.fn();
    const attachmentDrafts = createAttachmentDrafts([new File(["hello"], "note.txt", { type: "text/plain" })]);
    renderComposer({
      attachmentDrafts,
      draftText: "",
      onRemoveAttachment,
      onSendMessage,
    });

    expect(screen.getByText("note.txt")).toBeInTheDocument();
    const sendButton = screen.getByRole("button", { name: "Send" });
    expect(sendButton).not.toBeDisabled();

    expect(screen.getByText("note.txt")).toHaveAttribute("title", "note.txt");
    await user.click(screen.getByRole("button", { name: "Remove attachment: note.txt" }));
    expect(onRemoveAttachment).toHaveBeenCalledWith(attachmentDrafts[0].id);
    await user.click(sendButton);
    expect(onSendMessage).toHaveBeenCalledTimes(1);
  });

  it("offers mouse, keyboard, and wheel navigation when attachments overflow", async () => {
    const user = userEvent.setup();
    const attachmentDrafts = createAttachmentDrafts([
      new File(["one"], "one.txt", { type: "text/plain" }),
      new File(["two"], "two.txt", { type: "text/plain" }),
      new File(["three"], "three.txt", { type: "text/plain" }),
    ]);
    const { container } = renderComposer({ attachmentDrafts });
    const strip = container.querySelector<HTMLElement>(".attachment-draft-strip");
    expect(strip).not.toBeNull();
    Object.defineProperties(strip!, {
      clientWidth: { configurable: true, value: 240 },
      scrollLeft: { configurable: true, value: 0, writable: true },
      scrollWidth: { configurable: true, value: 700 },
    });

    fireEvent.scroll(strip!);
    expect(screen.getByRole("button", { name: "View more attachments" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "View previous attachments" })).not.toBeInTheDocument();
    expect(container.querySelector(".attachment-scroll-fade.is-next")).toBeInTheDocument();
    expect(container.querySelector(".attachment-scroll-fade.is-previous")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "View more attachments" }));
    expect(strip!.scrollLeft).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "View previous attachments" })).toBeInTheDocument();
    expect(container.querySelector(".attachment-scroll-fade.is-previous")).toBeInTheDocument();

    const scrollLeftBeforeWheel = strip!.scrollLeft;
    const wheelEvent = createEvent.wheel(strip!, { cancelable: true, deltaX: 0, deltaY: 40 });
    fireEvent(strip!, wheelEvent);
    expect(strip!.scrollLeft).toBeGreaterThan(scrollLeftBeforeWheel);
  });

  it("accepts files from the picker, paste, and drag and drop", async () => {
    const user = userEvent.setup();
    const onAddAttachments = vi.fn();
    const { container } = renderComposer({ onAddAttachments });
    const pickerFile = new File(["picker"], "picker.txt", { type: "text/plain" });
    const pastedFile = new File(["image"], "pasted.png", { type: "image/png" });
    const droppedFile = new File(["drop"], "dropped.pdf", { type: "application/pdf" });

    const input = container.querySelector<HTMLInputElement>('input[type="file"]');
    expect(input).not.toBeNull();
    await user.upload(input!, pickerFile);

    const editor = screen.getByRole("textbox", { name: "Message" });
    expect(editor).toHaveAttribute("aria-multiline", "true");
    fireEvent.paste(editor, {
      clipboardData: {
        files: [pastedFile],
        getData: () => "",
        items: [],
      },
    });

    const composerBox = container.querySelector<HTMLElement>(".composer-box");
    expect(composerBox).not.toBeNull();
    const dragData = {
      dropEffect: "none",
      files: [],
      items: [],
      types: ["Files"],
    };
    const dragOverEvent = createEvent.dragOver(composerBox!, { dataTransfer: dragData });
    fireEvent(composerBox!, dragOverEvent);
    expect(dragOverEvent.defaultPrevented).toBe(true);
    expect(dragData.dropEffect).toBe("copy");

    fireEvent.drop(composerBox!, {
      dataTransfer: {
        files: [droppedFile],
        items: [],
      },
    });

    expect(onAddAttachments).toHaveBeenNthCalledWith(1, [pickerFile]);
    expect(onAddAttachments).toHaveBeenNthCalledWith(2, [pastedFile]);
    expect(onAddAttachments).toHaveBeenNthCalledWith(3, [droppedFile]);
  });

  it("starts GitHub authorization from the dropdown row connect action", async () => {
    const user = userEvent.setup();
    const onConnectConnector = vi.fn();
    renderComposer({
      connectorStatus: {
        ...emptyGitHubConnectorStatus(),
        configured: true,
        connected: false,
        client_id: "client-id",
        client_secret_set: true,
        scopes: ["repo"],
      },
      onConnectConnector,
    });

    await user.click(screen.getByRole("button", { name: "Add content" }));
    const githubRow = screen.getByText("GitHub").closest(".connector-provider-row") as HTMLElement;
    await user.click(within(githubRow).getByRole("button", { name: "Connect" }));

    expect(onConnectConnector).toHaveBeenCalledTimes(1);
  });

  it("shows a green Connected state after GitHub is connected", async () => {
    const user = userEvent.setup();
    const onDisconnectConnector = vi.fn();
    const onManageConnector = vi.fn();
    renderComposer({
      connectorStatus: {
        ...emptyGitHubConnectorStatus(),
        configured: true,
        connected: true,
        app_manageable: true,
        client_id: "client-id",
        client_secret_set: true,
        scopes: ["repo"],
        account: {
          avatar_url: "https://github.com/images/error/octocat_happy.gif",
          email: "",
          html_url: "https://github.com/octocat",
          id: 583231,
          login: "octocat",
          name: "",
        },
      },
      onDisconnectConnector,
      onManageConnector,
    });

    await user.click(screen.getByRole("button", { name: "Add content" }));

    expect(screen.getByText("octocat")).toBeInTheDocument();
    const connectedState = screen.getByText("Connected");
    expect(connectedState).toBeInTheDocument();
    expect(connectedState).toHaveClass("connector-connected-state");
    const actions = connectedState.closest(".connector-provider-actions");
    expect(actions?.children[0]).toHaveTextContent("Connected");
    expect(actions?.children[1]).toHaveTextContent("Manage");
    expect(actions?.children[2]).toHaveTextContent("Disconnect");
    const manageButton = screen.getByRole("button", { name: "Manage" });
    expect(manageButton).toHaveClass("connector-manage-button");
    expect(manageButton).toHaveClass("btn-secondary-gray");
    expect(manageButton).not.toHaveClass("btn-tertiary-gray");
    const disconnectButton = screen.getByRole("button", { name: "Disconnect" });
    expect(disconnectButton).toHaveClass("connector-disconnect-button-danger");
    expect(disconnectButton).toHaveClass("btn-outline-danger");
    await user.click(manageButton);
    expect(onManageConnector).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole("button", { name: "Disconnect" }));
    expect(onDisconnectConnector).toHaveBeenCalledTimes(1);
    const githubRow = screen.getByText("GitHub").closest(".connector-provider-row") as HTMLElement;
    expect(within(githubRow).queryByRole("button", { name: "Connect" })).not.toBeInTheDocument();
    expect(screen.queryByText("secret")).not.toBeInTheDocument();
    expect(screen.queryByText("access_token")).not.toBeInTheDocument();
  });
});

import { createRef } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConversationComposer } from "@/components/business/ConversationPane/ConversationComposer";
import type { ConversationComposerProps } from "@/components/business/ConversationPane/ConversationComposer";
import { createAttachmentDrafts } from "@/models/attachments";
import { emptyGitHubConnectorStatus } from "@/models/connectors";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key) => {
  const labels: Record<string, string> = {
    connectorCallbackURL: "Callback URL",
    connectorClientID: "Client ID",
    connectorClientSecret: "Client Secret",
    connectorConnect: "Connect",
    connectorConnected: "Connected",
    connectorDisconnect: "Disconnect",
    connectorGitHub: "GitHub",
    connectorManage: "Manage",
    connectorManagerTitle: "Manage connectors",
    connectorNotConnected: "Not connected",
    connectorSave: "Save",
    connectorScopes: "Scopes",
    connectorSetUp: "Set up",
    composerAdd: "Add",
    composerFiles: "Files",
    composerConnectors: "Connectors",
    inputPlaceholder: "Message",
    addAttachment: "Add attachment",
    attachments: "Attachments",
    removeAttachment: "Remove attachment",
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
    expect(screen.getByLabelText("Message")).toHaveAttribute("contenteditable", "false");
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

    const button = screen.getByRole("button", { name: "Add" });
    expect(button).toHaveClass("composer-add-button");

    await user.click(button);

    const dialog = screen.getByRole("dialog", { name: "Add" });
    expect(dialog).toBeInTheDocument();
    expect(within(dialog).getByText("Files")).toBeInTheDocument();
    expect(within(dialog).getByText("Connectors")).toBeInTheDocument();
    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("Not connected")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Connect" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Client ID")).not.toBeInTheDocument();
    expect(screen.queryByText("Save")).not.toBeInTheDocument();
    expect(screen.queryByText("Set up")).not.toBeInTheDocument();
  });

  it("places the add and send controls in one composer toolbar", () => {
    const { container } = renderComposer();

    const row = container.querySelector(".composer-toolbar");
    expect(row).toBeInTheDocument();
    expect(within(row as HTMLElement).getByRole("button", { name: "Add" })).toHaveClass("composer-add-button");
    expect(within(row as HTMLElement).getByRole("button", { name: "Send" })).toHaveClass("composer-send-button");
    expect(container.querySelector(".composer-tip")).not.toBeInTheDocument();
  });

  it("opens the file picker from the Files menu item", async () => {
    const user = userEvent.setup();
    const inputClick = vi.spyOn(HTMLInputElement.prototype, "click");
    renderComposer();

    await user.click(screen.getByRole("button", { name: "Add" }));
    await user.click(screen.getByRole("button", { name: "Files" }));

    expect(inputClick).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog", { name: "Add" })).not.toBeInTheDocument();
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

    await user.click(screen.getByRole("button", { name: "Remove attachment" }));
    expect(onRemoveAttachment).toHaveBeenCalledWith(attachmentDrafts[0].id);
    await user.click(sendButton);
    expect(onSendMessage).toHaveBeenCalledTimes(1);
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

    const editor = screen.getByLabelText("Message");
    fireEvent.paste(editor, {
      clipboardData: {
        files: [pastedFile],
        getData: () => "",
        items: [],
      },
    });

    const composerBox = container.querySelector<HTMLElement>(".composer-box");
    expect(composerBox).not.toBeNull();
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

    await user.click(screen.getByRole("button", { name: "Add" }));
    await user.click(screen.getByRole("button", { name: "Connect" }));

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

    await user.click(screen.getByRole("button", { name: "Add" }));

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
    expect(screen.queryByRole("button", { name: "Connect" })).not.toBeInTheDocument();
    expect(screen.queryByText("secret")).not.toBeInTheDocument();
    expect(screen.queryByText("access_token")).not.toBeInTheDocument();
  });
});

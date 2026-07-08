import { createRef } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConversationComposer } from "@/components/business/ConversationPane/ConversationComposer";
import type { ConversationComposerProps } from "@/components/business/ConversationPane/ConversationComposer";
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
    composerTip: "Press Enter to send",
    inputPlaceholder: "Message",
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

  it("opens a Manus-style connector dropdown from the top-level connector icon", async () => {
    const user = userEvent.setup();
    renderComposer({
      connectorStatus: {
        ...emptyGitHubConnectorStatus(),
        configured: true,
        client_id: "client-id",
        client_secret_set: true,
      },
    });

    const button = screen.getByRole("button", { name: "Manage connectors" });
    expect(button).toHaveClass("composer-tool-button");
    expect(button).not.toHaveClass("composer-github-button");

    await user.click(button);

    const dialog = screen.getByRole("dialog", { name: "Manage connectors" });
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText("GitHub")).toBeInTheDocument();
    expect(screen.getByText("Not connected")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Connect" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Client ID")).not.toBeInTheDocument();
    expect(screen.queryByText("Save")).not.toBeInTheDocument();
    expect(screen.queryByText("Set up")).not.toBeInTheDocument();
  });

  it("places the composer hint to the right of the GitHub connector button", () => {
    const { container } = renderComposer();

    const row = container.querySelector(".composer-actions-row");
    expect(row).toBeInTheDocument();
    expect(row?.children[0]).toHaveClass("composer-connector-menu");
    expect(row?.children[1]).toHaveClass("composer-tip");
    expect(screen.getByRole("button", { name: "Manage connectors" })).toBeInTheDocument();
    expect(row).toHaveTextContent("Press Enter to send");
    expect(container.querySelector("footer > .composer-tip")).not.toBeInTheDocument();
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

    await user.click(screen.getByRole("button", { name: "Manage connectors" }));
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

    await user.click(screen.getByRole("button", { name: "Manage connectors" }));

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

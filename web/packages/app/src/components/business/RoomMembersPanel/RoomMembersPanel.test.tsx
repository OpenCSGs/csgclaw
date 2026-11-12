import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { RoomMembersPanel, type RoomMembersPanelProps } from "./RoomMembersPanel";

const worker = { id: "worker", name: "Worker", avatar: "avatar/3D-2.png" };
const source = { id: "source", name: "GitLab", participants: [{ id: "p-source", type: "notification" }] };

function props(overrides: Partial<RoomMembersPanelProps> = {}): RoomMembersPanelProps {
  return {
    candidates: [worker, source],
    currentUserID: "admin",
    inviteUserIDs: [],
    members: [{ id: "admin", name: "Admin" }],
    onClose: vi.fn(),
    onInvite: vi.fn(),
    onInviteUserIDsChange: vi.fn(),
    onRemoveMember: vi.fn(),
    onExternalMembershipChange: vi.fn(),
    onManageExternalMessages: vi.fn(),
    t: (key) => key,
    ...overrides,
  };
}

describe("RoomMembersPanel", () => {
  it("selects regular candidates without adding external sources to the invitation", async () => {
    const user = userEvent.setup();
    function Harness() {
      const [selected, setSelected] = useState<string[]>([]);
      return <RoomMembersPanel {...props({ inviteUserIDs: selected, onInviteUserIDsChange: setSelected })} />;
    }
    render(<Harness />);
    const regular = screen.getByRole("region", { name: "addMembers" });
    expect(within(regular).queryByText("GitLab")).not.toBeInTheDocument();
    await user.click(screen.getByRole("checkbox", { name: "allMembers" }));
    expect(screen.getByRole("checkbox", { name: "Worker" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "GitLab · externalMessagesDisconnected" })).not.toBeChecked();
    expect(screen.getByRole("button", { name: "inviteSelectedMembers" })).toBeEnabled();
    expect(document.querySelector(".agent-avatar-image")).toHaveAttribute("src", worker.avatar);
  });

  it("updates source membership immediately and opens its existing settings", async () => {
    const user = userEvent.setup();
    const panelProps = props();
    const { rerender } = render(<RoomMembersPanel {...panelProps} />);
    await user.click(screen.getByRole("switch", { name: "GitLab · externalMessagesDisconnected" }));
    expect(panelProps.onExternalMembershipChange).toHaveBeenCalledWith("source", true);
    expect(panelProps.onInvite).not.toHaveBeenCalled();
    rerender(<RoomMembersPanel {...panelProps} candidates={[worker]} members={[source]} />);
    await user.click(screen.getByRole("switch", { name: "GitLab · externalMessagesConnected" }));
    expect(panelProps.onExternalMembershipChange).toHaveBeenCalledWith("source", false);
    await user.click(screen.getByRole("button", { name: "externalMessagesManage" }));
    expect(panelProps.onManageExternalMessages).toHaveBeenCalledOnce();
  });

  it("keeps mutations unavailable while a request is pending and displays failures", () => {
    render(<RoomMembersPanel {...props({ memberActionBusyID: "source", memberActionError: "Request failed" })} />);
    expect(screen.getByRole("switch")).toBeDisabled();
    expect(screen.getByRole("checkbox", { name: "Worker" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "inviteSelectedMembers" })).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("Request failed");
    expect(screen.queryByRole("button", { name: "removeMember Admin" })).not.toBeInTheDocument();
  });

  it("makes the covered chat unavailable while preserving settings dialog Escape handling", async () => {
    const user = userEvent.setup();
    const panelProps = props();
    const { container, unmount } = render(
      <div>
        <button type="button" data-testid="chat-action">
          Chat action
        </button>
        <RoomMembersPanel {...panelProps} />
      </div>,
    );
    const chatAction = screen.getByTestId("chat-action");
    expect(chatAction.inert).toBe(true);
    const settingsDialog = document.createElement("div");
    settingsDialog.setAttribute("role", "dialog");
    document.body.append(settingsDialog);
    await user.keyboard("{Escape}");
    expect(panelProps.onClose).not.toHaveBeenCalled();
    settingsDialog.remove();
    await user.keyboard("{Escape}");
    expect(panelProps.onClose).toHaveBeenCalledOnce();
    expect(container.querySelector("aside")).toHaveAttribute("data-overlay", "true");
    unmount();
    expect(chatAction.inert).toBe(false);
  });
});

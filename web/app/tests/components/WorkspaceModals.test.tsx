import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { CreateRoomModal, CreateTeamModal, InviteMembersModal } from "@/pages/WorkspacePage/components/WorkspaceModals";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key) => key;

const avatarUser = {
  avatar: "avatar/3D-2.png",
  id: "u-avatar",
  name: "Avatar User",
};

describe("WorkspaceModals", () => {
  it("renders create-room member avatars from user avatar paths", () => {
    const { container } = render(
      <CreateRoomModal
        candidates={[avatarUser]}
        lockedRoomMemberIDs={[]}
        onClose={() => {}}
        onCreate={() => {}}
        onRoomDescriptionChange={() => {}}
        onRoomMemberIDsChange={() => {}}
        onRoomTitleChange={() => {}}
        roomDescription=""
        roomMemberIDs={[]}
        roomType="on_demand"
        roomTitle=""
        submitError=""
        t={t}
        onRoomTypeChange={() => {}}
      />,
    );

    expect(container.querySelector(".create-room-avatar .agent-avatar-image")).toHaveAttribute(
      "src",
      avatarUser.avatar,
    );
  });

  it("renders invite member avatars from user avatar paths", () => {
    const { container } = render(
      <InviteMembersModal
        candidates={[avatarUser]}
        currentUserID="u-test"
        members={[avatarUser]}
        allowMemberRemoval={false}
        inviteUserIDs={[]}
        onClose={() => {}}
        onInvite={() => {}}
        onInviteUserIDsChange={() => {}}
        submitError=""
        t={t}
      />,
    );

    expect(container.querySelector(".create-room-avatar .agent-avatar-image")).toHaveAttribute(
      "src",
      avatarUser.avatar,
    );
  });

  it("forwards Windows and IME-compatible team name change events", () => {
    const onTeamTitleChange = vi.fn();
    render(
      <CreateTeamModal
        candidates={[]}
        onClose={() => {}}
        onCreate={async () => {}}
        onTeamMemberIDsChange={() => {}}
        onTeamTitleChange={onTeamTitleChange}
        submitError=""
        t={t}
        teamActionBusy={false}
        teamMemberIDs={[]}
        teamTitle=""
      />,
    );

    fireEvent.change(screen.getByPlaceholderText("teamNamePlaceholder"), {
      target: { value: "Windows 测试团队" },
    });

    expect(onTeamTitleChange).toHaveBeenCalledWith("Windows 测试团队");
  });

  it("keeps locked team members selected when toggling all members", () => {
    function Harness() {
      const [teamMemberIDs, setTeamMemberIDs] = useState(["agent-manager"]);
      return (
        <CreateTeamModal
          candidates={[
            { id: "agent-manager", name: "Manager", role: "manager" },
            { id: "agent-worker", name: "Worker", role: "worker" },
          ]}
          lockedTeamMemberIDs={["agent-manager"]}
          onClose={() => {}}
          onCreate={async () => {}}
          onTeamMemberIDsChange={setTeamMemberIDs}
          onTeamTitleChange={() => {}}
          submitError=""
          t={t}
          teamActionBusy={false}
          teamMemberIDs={teamMemberIDs}
          teamTitle="Team"
        />
      );
    }

    render(<Harness />);
    const [allMembers, manager, worker] = screen.getAllByRole("checkbox");

    expect(manager).toBeChecked();
    expect(manager).toBeDisabled();
    expect(worker).not.toBeChecked();

    fireEvent.click(allMembers);
    expect(worker).toBeChecked();

    fireEvent.click(allMembers);
    expect(manager).toBeChecked();
    expect(worker).not.toBeChecked();
  });
});

import { fireEvent, render, screen } from "@testing-library/react";
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
        roomTitle=""
        submitError=""
        t={t}
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
});

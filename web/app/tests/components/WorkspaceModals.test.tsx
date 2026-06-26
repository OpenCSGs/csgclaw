import { render } from "@testing-library/react";
import { CreateRoomModal, InviteMembersModal } from "@/pages/WorkspacePage/components/WorkspaceModals";
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
});

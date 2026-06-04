import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { ProfilePreviewPopover } from "../ProfilePreviewPopover";
import {
  AgentProfileModal,
  CreateRoomModal,
  CreateTeamModal,
  InviteMembersModal,
  ManagerProfileSetupModal,
  ManagerRebuildModal,
  UpgradeModal,
} from "../WorkspaceModals";

export function WorkspaceOverlays() {
  const controller = useWorkspaceControllerContext();

  return (
    <>
      {controller.profilePreviewProps ? <ProfilePreviewPopover {...controller.profilePreviewProps} /> : null}
      {controller.createRoomModalProps ? <CreateRoomModal {...controller.createRoomModalProps} /> : null}
      {controller.createTeamModalProps ? <CreateTeamModal {...controller.createTeamModalProps} /> : null}
      {controller.inviteMembersModalProps ? <InviteMembersModal {...controller.inviteMembersModalProps} /> : null}
      {controller.upgradeModalProps ? <UpgradeModal {...controller.upgradeModalProps} /> : null}
      {controller.agentProfileModalProps ? <AgentProfileModal {...controller.agentProfileModalProps} /> : null}
      {controller.managerRebuildModalProps ? <ManagerRebuildModal {...controller.managerRebuildModalProps} /> : null}
      {controller.managerProfileSetupModalProps ? (
        <ManagerProfileSetupModal {...controller.managerProfileSetupModalProps} />
      ) : null}
    </>
  );
}

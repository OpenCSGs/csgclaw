import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { FloatingChat } from "../FloatingChat";
import { ProfilePreviewPopover } from "../ProfilePreviewPopover";
import {
  AgentProfileModal,
  CreateRoomModal,
  CreateTeamModal,
  InviteMembersModal,
  ManagerRebuildModal,
  UpgradeModal,
  ConfigSettingsModal,
} from "../WorkspaceModals";

export function WorkspaceOverlays() {
  const controller = useWorkspaceControllerContext();

  return (
    <>
      {controller.floatingChatProps ? <FloatingChat {...controller.floatingChatProps} /> : null}
      {controller.profilePreviewProps ? <ProfilePreviewPopover {...controller.profilePreviewProps} /> : null}
      {controller.createRoomModalProps ? <CreateRoomModal {...controller.createRoomModalProps} /> : null}
      {controller.createTeamModalProps ? <CreateTeamModal {...controller.createTeamModalProps} /> : null}
      {controller.inviteMembersModalProps ? <InviteMembersModal {...controller.inviteMembersModalProps} /> : null}
      {controller.upgradeModalProps ? <UpgradeModal {...controller.upgradeModalProps} /> : null}
      {controller.configModalProps ? <ConfigSettingsModal {...controller.configModalProps} /> : null}
      {controller.agentProfileModalProps ? <AgentProfileModal {...controller.agentProfileModalProps} /> : null}
      {controller.managerRebuildModalProps ? <ManagerRebuildModal {...controller.managerRebuildModalProps} /> : null}
    </>
  );
}

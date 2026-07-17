import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { workspaceShowsFloatingChat } from "@/models/routing";
import { AuthLoginNotice } from "../AuthLoginNotice";
import { FloatingChat } from "../FloatingChat";
import { ProfilePreviewPopover } from "../ProfilePreviewPopover";
import {
  AgentProfileModal,
  CreateModelProviderModal,
  CreateRoomModal,
  CreateTeamModal,
  InviteMembersModal,
  UpgradeModal,
  ConfigSettingsModal,
} from "../WorkspaceModals";

export function WorkspaceOverlays() {
  const controller = useWorkspaceControllerContext();
  const closeLabel = controller.sidebarProps?.t?.("close") || "Close";
  const showFloatingChat = workspaceShowsFloatingChat(controller.activePane);

  return (
    <>
      <AuthLoginNotice
        notice={controller.authNotice}
        closeLabel={closeLabel}
        onDismiss={controller.onDismissAuthNotice}
      />
      {showFloatingChat && controller.floatingChatProps ? <FloatingChat {...controller.floatingChatProps} /> : null}
      {controller.profilePreviewProps ? <ProfilePreviewPopover {...controller.profilePreviewProps} /> : null}
      {controller.createRoomModalProps ? <CreateRoomModal {...controller.createRoomModalProps} /> : null}
      {controller.createModelProviderModalProps ? (
        <CreateModelProviderModal {...controller.createModelProviderModalProps} />
      ) : null}
      {controller.createTeamModalProps ? <CreateTeamModal {...controller.createTeamModalProps} /> : null}
      {controller.inviteMembersModalProps ? <InviteMembersModal {...controller.inviteMembersModalProps} /> : null}
      {controller.upgradeModalProps ? <UpgradeModal {...controller.upgradeModalProps} /> : null}
      {controller.configModalProps ? <ConfigSettingsModal {...controller.configModalProps} /> : null}
      {controller.agentProfileModalProps ? <AgentProfileModal {...controller.agentProfileModalProps} /> : null}
    </>
  );
}

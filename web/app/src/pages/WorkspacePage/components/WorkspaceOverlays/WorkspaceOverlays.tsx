import { OpenCSGConnectionDialog } from "@/components/business/OpenCSGConnectionDialog";
import { lazy, Suspense } from "react";
import { useWorkspaceDialogs } from "@/hooks/workspace/useWorkspaceDialogs";
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

const SettingsPage = lazy(() => import("@/pages/SettingsPage").then((module) => ({ default: module.SettingsPage })));
const HumanPage = lazy(() => import("@/pages/HumanPage").then((module) => ({ default: module.HumanPage })));

export function WorkspaceOverlays() {
  const controller = useWorkspaceControllerContext();
  const { current } = useWorkspaceDialogs();
  const closeLabel = controller.sidebarProps?.t?.("close") || "Close";
  const showFloatingChat =
    workspaceShowsFloatingChat(controller.activePane) &&
    !(controller.ready && controller.taskEntry && controller.activePane.type === "conversation");

  return (
    <>
      <Suspense fallback={null}>
        {current === "settings" ? <SettingsPage /> : null}
        {current === "profile" ? <HumanPage /> : null}
      </Suspense>
      <AuthLoginNotice
        notice={controller.authNotice}
        closeLabel={closeLabel}
        onDismiss={controller.onDismissAuthNotice}
      />
      {controller.openCSGLoginRequiredDialogProps ? (
        <OpenCSGConnectionDialog {...controller.openCSGLoginRequiredDialogProps} />
      ) : null}
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

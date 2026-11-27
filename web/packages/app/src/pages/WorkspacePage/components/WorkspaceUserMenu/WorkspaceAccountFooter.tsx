import { useWorkspaceDialogs } from "@/hooks/workspace";
import type { WorkspaceSidebarProps } from "@/hooks/workspace/contracts/sidebar";
import { latestSidebarUpdateVersion } from "@/models/upgradeStatus";
import { SidebarUpgradeButton } from "../SidebarUpgradeButton";
import { WorkspaceUserMenu } from "./WorkspaceUserMenu";
import styles from "./WorkspaceUserMenu.module.css";

type FooterProps = Pick<
  WorkspaceSidebarProps,
  | "authStatus"
  | "authExpired"
  | "theme"
  | "locale"
  | "onThemeChange"
  | "onLocaleChange"
  | "onLogout"
  | "onOpenSettings"
  | "showUpgradeControls"
  | "appVersion"
  | "upgradeStatus"
  | "upgradePhase"
  | "currentUserID"
  | "usersById"
  | "isSidebarCollapsed"
  | "t"
> & { onSelectDestination?: (action: () => void) => void };

export function WorkspaceAccountFooter({ onSelectDestination = (action) => action(), ...props }: FooterProps) {
  const { openProfile, openSettings, setFeedbackOpen } = useWorkspaceDialogs();
  const { t, isSidebarCollapsed } = props;
  const updateVersion = latestSidebarUpdateVersion(props);

  return (
    <div className={styles.footer}>
      {updateVersion ? (
        <SidebarUpgradeButton
          collapsed={isSidebarCollapsed}
          version={updateVersion}
          t={t}
          onClick={() => onSelectDestination(() => openSettings("version", true))}
        />
      ) : null}
      <WorkspaceUserMenu
        currentUser={props.usersById.get(props.currentUserID)}
        openCSG={{
          authenticated: props.authStatus.authenticated === true,
          expired: props.authExpired,
          baseURL: props.authStatus.base_url,
        }}
        theme={props.theme}
        locale={props.locale}
        t={t}
        presentation={isSidebarCollapsed ? "icon" : "row"}
        onThemeChange={props.onThemeChange}
        onLocaleChange={props.onLocaleChange}
        onOpenSettings={() => onSelectDestination(props.onOpenSettings)}
        onOpenProfile={() => onSelectDestination(() => openProfile(props.currentUserID))}
        onOpenCSGLogin={() => onSelectDestination(() => openSettings("account"))}
        onOpenCSGLogout={() => void props.onLogout()}
        onOpenFeedback={(trigger) => onSelectDestination(() => setFeedbackOpen(true, trigger))}
      />
    </div>
  );
}

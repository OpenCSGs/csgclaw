import { ArrowRight, CircleArrowUp } from "lucide-react";
import { Tooltip } from "@/components/ui";
import { useWorkspaceDialogs } from "@/hooks/workspace";
import type { WorkspaceSidebarProps } from "@/hooks/workspace/contracts/sidebar";
import { formatSidebarVersionLabel, isLocalBuildUpgradeStatus } from "@/models/upgradeStatus";
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
  | "upgradeStatus"
  | "upgradePhase"
  | "currentUserID"
  | "usersById"
  | "isSidebarCollapsed"
  | "t"
> & { onSelectDestination?: (action: () => void) => void };

export function WorkspaceAccountFooter({ onSelectDestination = (action) => action(), ...props }: FooterProps) {
  const { openProfile, openSettings, setFeedbackOpen } = useWorkspaceDialogs();
  const { upgradeStatus, t, isSidebarCollapsed } = props;
  const latest = upgradeStatus?.latest_version;
  const upgradeAvailable =
    props.showUpgradeControls &&
    latest &&
    upgradeStatus.update_available &&
    upgradeStatus.auto_upgrade_supported !== false &&
    !isLocalBuildUpgradeStatus(upgradeStatus, upgradeStatus.current_version) &&
    props.upgradePhase !== "done";
  const label = t("sidebarNewVersion", { version: formatSidebarVersionLabel(latest) });

  return (
    <div className={styles.footer}>
      {upgradeAvailable ? (
        <Tooltip content={isSidebarCollapsed ? label : null}>
          <button
            type="button"
            className={styles.upgradeLink}
            aria-label={label}
            onClick={() => onSelectDestination(() => openSettings("version", true))}
          >
            {isSidebarCollapsed ? (
              <CircleArrowUp size={18} aria-hidden="true" />
            ) : (
              <>
                {label} <ArrowRight size={14} aria-hidden="true" />
              </>
            )}
          </button>
        </Tooltip>
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
        onOpenFeedback={() => onSelectDestination(() => setFeedbackOpen(true))}
      />
    </div>
  );
}

import { Tooltip } from "@/components/ui";
import { SidebarGearIcon } from "@/components/ui/Icons";
import type { TranslateFn } from "@/models/conversations";
import type { UpgradePhase, UpgradeStatus } from "@/models/upgradeStatus";
import { isLocalBuildUpgradeStatus } from "@/models/upgradeStatus";
import { classNames } from "@/shared/lib/classNames";
import { shouldShowUpgradeAlertDot } from "./sidebarUpgradeAlert";
import styles from "./SidebarUserButton.module.css";

type SidebarUserButtonProps = {
  active?: boolean;
  presentation?: "icon" | "row";
  appVersion?: string;
  upgradeStatus?: UpgradeStatus | null;
  upgradeBusy?: boolean;
  upgradePhase?: UpgradePhase;
  showUpgradeControls?: boolean;
  onOpenSettings: () => void;
  t: TranslateFn;
};

export function SidebarUserButton({
  active = false,
  presentation = "icon",
  appVersion = "",
  upgradeStatus = null,
  upgradeBusy = false,
  upgradePhase = "idle",
  showUpgradeControls = true,
  onOpenSettings,
  t,
}: SidebarUserButtonProps) {
  const controlsAvailable =
    showUpgradeControls &&
    upgradeStatus?.auto_upgrade_supported !== false &&
    !isLocalBuildUpgradeStatus(upgradeStatus, upgradeStatus?.current_version || appVersion);
  const upgradeAttention = shouldShowUpgradeAlertDot({
    controlsAvailable,
    phase: upgradePhase,
    busy: upgradeBusy,
    status: upgradeStatus,
  });
  return (
    <div className={classNames(styles.root, presentation === "row" ? styles.rootRow : styles.rootIcon)}>
      <Tooltip content={presentation === "icon" ? t("settings") : null}>
        <button
          type="button"
          className={classNames(
            styles.button,
            presentation === "row" ? styles.buttonRow : styles.buttonIcon,
            active && styles.active,
          )}
          aria-label={t("settings")}
          aria-current={active ? "page" : undefined}
          onClick={onOpenSettings}
        >
          <span className={styles.settingsMark} aria-hidden="true">
            <SidebarGearIcon size={24} />
          </span>
          {presentation === "row" ? <span className={styles.buttonLabel}>{t("settings")}</span> : null}
          {upgradeAttention ? <span className={styles.alertDot} aria-hidden="true" /> : null}
        </button>
      </Tooltip>
    </div>
  );
}

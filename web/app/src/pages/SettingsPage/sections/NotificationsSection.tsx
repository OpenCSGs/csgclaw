import { Button, Select } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { classNames } from "@/shared/lib/classNames";
import {
  DEFAULT_TURN_NOTIFICATION_MODE,
  normalizeTurnNotificationMode,
  TurnNotificationModes,
} from "@/models/turnNotifications";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";

export function NotificationsSection() {
  const controller = useWorkspaceControllerContext();
  const sidebar = controller.sidebarProps;
  if (!sidebar) {
    return null;
  }
  const t = sidebar.t;

  const turnNotificationMode = sidebar.turnNotificationMode ?? DEFAULT_TURN_NOTIFICATION_MODE;
  const turnNotificationPermission = sidebar.turnNotificationPermission ?? "unsupported";
  const turnNotificationOptions = [
    { label: t("turnNotificationModeOff"), value: TurnNotificationModes.off },
    { label: t("turnNotificationModeAlways"), value: TurnNotificationModes.always },
    { label: t("turnNotificationModeWhenUnfocused"), value: TurnNotificationModes.whenUnfocused },
  ];
  const selectedTurnNotificationLabel =
    turnNotificationOptions.find((option) => option.value === turnNotificationMode)?.label ??
    t("turnNotificationModeWhenUnfocused");

  return (
    <SettingsRow title={t("notificationSettings")} description={t("settingsNotificationDescription")}>
      <div className={styles.formGrid}>
        <label className="field">
          <span>{t("turnCompletionNotifications")}</span>
          <Select
            contentClassName={styles.notificationSelectContent}
            options={turnNotificationOptions}
            selectedLabel={selectedTurnNotificationLabel}
            triggerClassName={styles.notificationSelectTrigger}
            triggerProps={{ "aria-label": t("turnCompletionNotifications") }}
            value={turnNotificationMode}
            onValueChange={(value) => void sidebar.onTurnNotificationModeChange?.(normalizeTurnNotificationMode(value))}
          />
        </label>
        <div className="field">
          <span>{t("notificationPermission")}</span>
          <div className={styles.notificationPermissionControls}>
            <span
              className={classNames(
                styles.permissionStatus,
                turnNotificationPermission === "granted" && styles.permissionGranted,
                turnNotificationPermission === "denied" && styles.permissionDenied,
              )}
            >
              <span aria-hidden="true" />
              {t(`notificationPermission${permissionLabelSuffix(turnNotificationPermission)}`)}
            </span>
            {turnNotificationPermission === "default" ? (
              <Button
                className={styles.designButton}
                variant="secondaryGray"
                size="md"
                onClick={() => void sidebar.onRequestTurnNotificationPermission?.()}
              >
                {t("notificationPermissionEnable")}
              </Button>
            ) : null}
          </div>
        </div>
        {turnNotificationPermission === "denied" ? (
          <p className={classNames(styles.notificationPermissionHint, styles.spanAll)}>
            {t("notificationPermissionDeniedHint")}
          </p>
        ) : null}
      </div>
    </SettingsRow>
  );
}

function permissionLabelSuffix(permission: string): string {
  switch (permission) {
    case "granted":
      return "Granted";
    case "denied":
      return "Denied";
    case "default":
      return "Default";
    default:
      return "Unsupported";
  }
}

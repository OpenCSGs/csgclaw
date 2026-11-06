import { Button, Select, Switch } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { classNames } from "@/shared/lib/classNames";
import {
  DEFAULT_TURN_NOTIFICATION_MODE,
  normalizeTurnNotificationMode,
  TurnNotificationModes,
} from "@/models/turnNotifications";
import { SettingsRow } from "./SettingsRow";
import notificationsSectionStyles from "./NotificationsSection.module.css";

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
      <div className={notificationsSectionStyles.notificationControls}>
        <div className={notificationsSectionStyles.notificationModeControl}>
          <Select
            size="sm"
            contentClassName={notificationsSectionStyles.notificationSelectContent}
            options={turnNotificationOptions}
            selectedLabel={selectedTurnNotificationLabel}
            triggerClassName={notificationsSectionStyles.notificationSelectTrigger}
            triggerProps={{ "aria-label": t("turnCompletionNotifications") }}
            value={turnNotificationMode}
            onValueChange={(value) => void sidebar.onTurnNotificationModeChange?.(normalizeTurnNotificationMode(value))}
          />
          <p className={notificationsSectionStyles.notificationModeHint}>{t("notificationPermissionDeniedHint")}</p>
        </div>
        <label className={notificationsSectionStyles.notificationSoundControl} title={t("turnNotificationSoundHint")}>
          <span>{t("turnNotificationSound")}</span>
          <Switch
            aria-label={t("turnNotificationSound")}
            aria-description={t("turnNotificationSoundHint")}
            checked={sidebar.turnNotificationSound ?? true}
            onCheckedChange={(enabled) => sidebar.onTurnNotificationSoundChange?.(enabled)}
          />
        </label>
        <div className={notificationsSectionStyles.notificationPermissionControls}>
          <span
            role="status"
            aria-label={`${t("notificationPermission")}: ${t(`notificationPermission${permissionLabelSuffix(turnNotificationPermission)}`)}`}
            title={
              turnNotificationPermission === "denied"
                ? t("notificationPermissionDeniedHint")
                : t("notificationPermission")
            }
            className={classNames(
              notificationsSectionStyles.permissionStatus,
              turnNotificationPermission === "granted" && notificationsSectionStyles.permissionGranted,
              turnNotificationPermission === "denied" && notificationsSectionStyles.permissionDenied,
            )}
          >
            <span aria-hidden="true" />
            {t(`notificationPermission${permissionLabelSuffix(turnNotificationPermission)}`)}
          </span>
          {turnNotificationPermission === "default" ? (
            <Button
              variant="secondaryGray"
              size="sm"
              onClick={() => void sidebar.onRequestTurnNotificationPermission?.()}
            >
              {t("notificationPermissionEnable")}
            </Button>
          ) : null}
        </div>
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

import { useState } from "react";
import { OpenCSGSwitchDialog } from "@/components/business/OpenCSGConnectionDialog";
import { Button } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import {
  authEnvironmentDisplayLabel,
  authEnvironmentDraftFromStatus,
  defaultAuthEnvironmentDraft,
} from "@/models/authEnvironment";
import { SettingsRow } from "./SettingsRow";
import { useAuthLoginDialog } from "./useAuthLoginDialog";
import styles from "../SettingsPage.module.css";

export function AccountSection() {
  const controller = useWorkspaceControllerContext();
  const sidebar = controller.sidebarProps;
  const [switchOpen, setSwitchOpen] = useState(false);
  const { loginDialog, openLoginDialog } = useAuthLoginDialog();

  if (!sidebar) {
    return null;
  }

  const t = sidebar.t;
  const signedIn = isAuthenticated(sidebar.authStatus);
  const accountName =
    sidebar.authStatus.name || sidebar.authStatus.user_id || sidebar.authStatus.user_uuid || t("csghubSignedIn");
  const activeAuthEnvironmentDraft = authEnvironmentDraftFromStatus(
    sidebar.authStatus,
    sidebar.authEnvironment ?? defaultAuthEnvironmentDraft(),
  );
  const authEnvironmentLabel = authEnvironmentDisplayLabel(activeAuthEnvironmentDraft, t("csghubEnvCustom"));

  // Clear the current identity before authenticating against another site.
  async function handleSwitchEnvironment() {
    await sidebar?.onLogout();
    setSwitchOpen(false);
    openLoginDialog(activeAuthEnvironmentDraft);
  }

  return (
    <>
      <SettingsRow title={t("settingsOpenCSGAccount")} description={t("settingsOpenCSGDescription")}>
        <div className={`${styles.formGrid} ${styles.accountFormGrid}`}>
          <div className="field">
            <span>{t("currentAccount")}</span>
            <strong className={styles.fieldValue}>{signedIn ? accountName : t("csghubNotSignedIn")}</strong>
          </div>
          <div className="field">
            <span>{t("settingsEnvironmentLabel")}</span>
            <strong className={styles.fieldValue}>{signedIn ? authEnvironmentLabel : "-"}</strong>
          </div>
          <div className={`${styles.formActions} ${styles.spanAll}`}>
            {sidebar.authExpired ? (
              <Button
                variant="primary"
                size="md"
                disabled={sidebar.authBusy}
                onClick={() => openLoginDialog(activeAuthEnvironmentDraft)}
              >
                {t("csghubReauthorize")}
              </Button>
            ) : null}
            {signedIn ? (
              <>
                <Button
                  variant="secondaryGray"
                  size="md"
                  disabled={sidebar.authBusy}
                  onClick={() => setSwitchOpen(true)}
                >
                  {t("csghubSwitchEnvironment")}
                </Button>
                <Button
                  variant="secondaryGray"
                  size="md"
                  disabled={sidebar.authBusy}
                  loading={sidebar.authLoggingOut}
                  loadingLabel={t("csghubSigningOut")}
                  onClick={() => void sidebar.onLogout()}
                >
                  {t("csghubSignOut")}
                </Button>
              </>
            ) : (
              <Button
                variant="primary"
                size="md"
                disabled={sidebar.authBusy}
                loading={sidebar.authBusy}
                loadingLabel={sidebar.authLoggingOut ? t("csghubSigningOut") : t("csghubSigningIn")}
                onClick={() => openLoginDialog()}
              >
                {sidebar.authLoggingOut
                  ? t("csghubSigningOut")
                  : sidebar.authPending
                    ? t("csghubReauthorize")
                    : t("settingsAccountLogin")}
              </Button>
            )}
          </div>
          {sidebar.authPending ? <p className={styles.statusHint}>{t("csghubLoginPendingDetail")}</p> : null}
          {sidebar.authError ? <p className={styles.inlineError}>{sidebar.authError}</p> : null}
        </div>
      </SettingsRow>

      {loginDialog}
      <OpenCSGSwitchDialog
        accountName={accountName}
        busy={sidebar.authBusy}
        environmentLabel={authEnvironmentLabel}
        open={switchOpen}
        t={t}
        onConfirm={() => void handleSwitchEnvironment()}
        onOpenChange={setSwitchOpen}
      />
    </>
  );
}

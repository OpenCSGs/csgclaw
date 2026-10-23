import { useEffect, useState } from "react";
import { Button } from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import { githubFeedbackIssueURL } from "@/models/feedback";
import { FeedbackDialog } from "../components/FeedbackDialog";
import { useFeedback } from "../useFeedback";
import { SettingsRow } from "./SettingsRow";
import { useAuthLoginDialog } from "./useAuthLoginDialog";
import styles from "../SettingsPage.module.css";
import { openOnboardingGuide } from "@/shared/lib/onboardingGuide";

export function FeedbackSection() {
  const controller = useWorkspaceControllerContext();
  const sidebar = controller.sidebarProps;
  const [feedbackOpen, setFeedbackOpen] = useState(false);
  const [feedbackPendingLogin, setFeedbackPendingLogin] = useState(false);
  const { loginDialog, openLoginDialog, setConnectionOpen } = useAuthLoginDialog();
  const feedback = useFeedback(
    controller.t,
    `${sidebar?.authStatus.user_uuid || ""}:${sidebar?.authStatus.base_url || ""}`,
  );

  const signedIn = isAuthenticated(sidebar?.authStatus);

  useEffect(() => {
    if (feedbackPendingLogin && signedIn) {
      setFeedbackPendingLogin(false);
      setConnectionOpen(false);
      setFeedbackOpen(true);
    }
  }, [feedbackPendingLogin, setConnectionOpen, signedIn]);

  if (!sidebar) {
    return null;
  }
  const t = sidebar.t;
  const feedbackURL = githubFeedbackIssueURL(sidebar.appVersion, sidebar.upgradeStatus);

  return (
    <>
      <SettingsRow title={t("configSettingsFeedbackSection")} description={t("settingsFeedbackDescription")}>
        <div className={`${styles.formActions} ${styles.spanAll}`}>
          <Button
            variant="primary"
            size="md"
            onClick={() => {
              if (signedIn) {
                setFeedbackOpen(true);
                return;
              }
              setFeedbackPendingLogin(true);
              openLoginDialog();
            }}
          >
            {t("feedbackWrite")}
          </Button>
          <a className={styles.linkButton} href={feedbackURL} target="_blank" rel="noreferrer">
            {t("settingsFeedbackGithubAction")}
          </a>
        </div>
      </SettingsRow>

      <SettingsRow title={t("onboardingGuideTitle")} description={t("onboardingGuideDescription")}>
        <div className={`${styles.formActions} ${styles.spanAll}`}>
          <Button variant="secondaryGray" size="md" onClick={openOnboardingGuide}>
            {t("onboardingGuideReplay")}
          </Button>
        </div>
      </SettingsRow>

      {feedback.success ? <p role="status">{t("feedbackSuccess")}</p> : null}

      <FeedbackDialog
        open={feedbackOpen}
        {...feedback}
        t={t}
        onOpenChange={setFeedbackOpen}
        onDescriptionChange={feedback.changeDescription}
        onImagesChange={feedback.changeImages}
        onSubmit={() => {
          void feedback.submit().then((success) => {
            if (success) {
              setFeedbackOpen(false);
            }
          });
        }}
      />
      {loginDialog}
    </>
  );
}

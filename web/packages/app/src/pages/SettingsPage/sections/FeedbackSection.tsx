import { Button } from "@/components/ui";
import { useWorkspaceControllerContext, useWorkspaceDialogs } from "@/hooks/workspace";
import { githubFeedbackIssueURL } from "@/models/feedback";
import { SettingsRow } from "./SettingsRow";
import styles from "../SettingsPage.module.css";
import accountSectionStyles from "./AccountSection.module.css";
import { openOnboardingGuide } from "@/shared/lib/onboardingGuide";

export function FeedbackSection() {
  const controller = useWorkspaceControllerContext();
  const sidebar = controller.sidebarProps;
  const { setFeedbackOpen } = useWorkspaceDialogs();

  if (!sidebar) {
    return null;
  }
  const t = sidebar.t;
  const feedbackURL = githubFeedbackIssueURL(sidebar.appVersion, sidebar.upgradeStatus);

  return (
    <>
      <SettingsRow title={t("configSettingsFeedbackSection")} description={t("settingsFeedbackDescription")}>
        <div className={`${styles.formActions} ${styles.spanAll}`}>
          <Button variant="primary" size="md" onClick={() => setFeedbackOpen(true)}>
            {t("feedbackWrite")}
          </Button>
          <a className={accountSectionStyles.linkButton} href={feedbackURL} target="_blank" rel="noreferrer">
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
    </>
  );
}

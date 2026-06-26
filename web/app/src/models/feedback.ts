import type { UpgradeStatus } from "@/models/upgradeStatus";
import { formatSidebarVersionLabel } from "@/models/upgradeStatus";

const GITHUB_ISSUE_CREATE_URL = "https://github.com/OpenCSGs/csgclaw/issues/new";
const GITHUB_FEEDBACK_LABEL = "user-feedback";

export function githubFeedbackIssueURL(appVersion: string, upgradeStatus: UpgradeStatus | null): string {
  const currentVersion = formatSidebarVersionLabel(upgradeStatus?.current_version || appVersion || "dev");
  const body = ["## Version information", `- CSGClaw version: ${currentVersion}`, ""].join("\n");
  const params = new URLSearchParams({
    body,
    labels: GITHUB_FEEDBACK_LABEL,
  });
  return `${GITHUB_ISSUE_CREATE_URL}?${params.toString()}`;
}

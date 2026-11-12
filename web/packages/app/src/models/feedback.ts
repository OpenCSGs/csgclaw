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

export const MAX_FEEDBACK_IMAGES = 6;
export const MAX_FEEDBACK_IMAGE_BYTES = 5 * 1024 * 1024;
export const MAX_FEEDBACK_DESCRIPTION_LENGTH = 20000;

export function feedbackImagesValid(images: Pick<File, "size" | "type">[]): boolean {
  return (
    images.length <= MAX_FEEDBACK_IMAGES &&
    images.every(
      (image) =>
        image.size > 0 && image.size <= MAX_FEEDBACK_IMAGE_BYTES && ["image/jpeg", "image/png"].includes(image.type),
    )
  );
}

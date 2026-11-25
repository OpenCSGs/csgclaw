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

export const MAX_FEEDBACK_ARCHIVES = 3;
export const MAX_FEEDBACK_ARCHIVE_BYTES = 10 * 1024 * 1024;
export const FEEDBACK_ATTACHMENT_ACCEPT = "image/jpeg,image/png,.jpg,.jpeg,.png,.zip,.rar,.7z,.tar,.gz";

const ARCHIVE_MIME_TYPES = [
  "application/zip",
  "application/x-zip-compressed",
  "application/vnd.rar",
  "application/x-rar-compressed",
  "application/x-7z-compressed",
  "application/x-tar",
  "application/gzip",
  "application/x-gzip",
];

export function feedbackAttachmentKind(file: File): "image" | "archive" | undefined {
  const extension = /\.([^.]+)$/.exec(file.name)?.[1].toLowerCase() || "";
  if (["image/jpeg", "image/png"].includes(file.type)) return "image";
  if (ARCHIVE_MIME_TYPES.includes(file.type)) return "archive";
  if (file.type && file.type !== "application/octet-stream") return;
  if (["jpg", "jpeg", "png"].includes(extension)) return "image";
  if (["zip", "rar", "7z", "tar", "gz"].includes(extension)) return "archive";
}

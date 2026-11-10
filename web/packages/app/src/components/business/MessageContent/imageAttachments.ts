import { resolveRequestPath } from "@/api/client";
import { isImageAttachment, type MessageAttachment } from "@/models/attachments";

export function resolveImageAttachmentURL(
  source: string,
  attachments: readonly MessageAttachment[],
): string | undefined {
  let path = source.trim().split(/[?#]/, 1)[0] || "";
  try {
    path = decodeURIComponent(path);
  } catch {
    // A published filename can contain a literal percent sign.
  }
  path = normalizePath(path);
  if (!path || path.startsWith("/") || /^[a-z][a-z\d+.-]*:/i.test(path)) {
    return undefined;
  }

  const images = attachments.filter(isImageAttachment);
  const exact = images.filter(
    (attachment) =>
      (attachment.source_path && normalizePath(attachment.source_path) === path) ||
      (attachment.workspace_path && normalizePath(attachment.workspace_path) === path),
  );
  const matches = exact.length ? exact : images.filter((attachment) => attachment.name === path);
  if (matches.length !== 1) {
    return undefined;
  }
  const attachment = matches[0];
  return resolveRequestPath(attachment.preview_url || attachment.download_url) || undefined;
}

function normalizePath(path: string): string {
  return path.replaceAll("\\", "/").replace(/^(?:\.\/)+/, "");
}

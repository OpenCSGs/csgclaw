import { resolveRequestPath } from "@/api/client";
import { isImageAttachment, type MessageAttachment } from "@/models/attachments";

export function resolveImageAttachmentURL(
  source: string,
  attachments: readonly MessageAttachment[],
): string | undefined {
  let reference = source.trim().split(/[?#]/, 1)[0] || "";
  if (/^[a-z][a-z\d+.-]*:/i.test(reference)) return undefined;
  try {
    reference = decodeURIComponent(reference);
  } catch {
    // A published filename can contain a literal percent sign.
  }
  const path = normalizePath(reference);
  if (!path) {
    return undefined;
  }

  const images = attachments.filter(isImageAttachment);
  const exact = images.filter(
    (attachment) =>
      (attachment.source_path && normalizePath(attachment.source_path) === path) ||
      (attachment.workspace_path && normalizePath(attachment.workspace_path) === path),
  );
  // A directory-qualified reference must match a recorded path, even if ".." reduces it to a filename.
  const name = reference.replace(/^(?:\.[/\\])+/, "");
  const matches = exact.length ? exact : images.filter((attachment) => attachment.name === name);
  if (matches.length !== 1) {
    return undefined;
  }
  const attachment = matches[0];
  return resolveRequestPath(attachment.preview_url || attachment.download_url) || undefined;
}

function normalizePath(path: string): string | undefined {
  const relative = path.replaceAll("\\", "/");
  if (relative.startsWith("/") || /^[a-z]:\//i.test(relative)) return undefined;

  const segments: string[] = [];
  for (const segment of relative.split("/")) {
    if (!segment || segment === ".") continue;
    if (segment === "..") {
      if (segments.length === 0) return undefined;
      segments.pop();
    } else {
      segments.push(segment);
    }
  }
  return segments.join("/") || undefined;
}

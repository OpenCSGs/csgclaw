export type AttachmentKind = "image" | "file";

export const MAX_ATTACHMENTS_PER_MESSAGE = 10;
export const MAX_ATTACHMENT_FILE_BYTES = 25 * 1024 * 1024;
export const MAX_ATTACHMENT_MESSAGE_BYTES = 64 * 1024 * 1024;

let attachmentDraftSequence = 0;

export type AttachmentDraft = {
  file: File;
  id: string;
  kind: AttachmentKind;
  mediaType: string;
  name: string;
  sizeBytes: number;
};

export type MessageAttachment = {
  created_at: string;
  download_url: string;
  height?: number;
  id: string;
  kind: AttachmentKind | string;
  media_type: string;
  name: string;
  preview_url?: string;
  sha256: string;
  size_bytes: number;
  width?: number;
  workspace_path?: string;
};

export type AttachmentSelectionResult = {
  countExceeded: boolean;
  fileTooLarge: boolean;
  files: File[];
  totalTooLarge: boolean;
};

export function createAttachmentDrafts(files: Iterable<File>, existingCount = 0): AttachmentDraft[] {
  return Array.from(files)
    .filter((file) => file.size > 0)
    .map((file, index) => {
      const mediaType = String(file.type || "application/octet-stream").trim() || "application/octet-stream";
      return {
        file,
        id: `draft-${Date.now()}-${existingCount + index}-${++attachmentDraftSequence}-${sanitizeDraftIDPart(file.name)}`,
        kind: attachmentKindFromMediaType(mediaType),
        mediaType,
        name: file.name || "attachment",
        sizeBytes: file.size,
      };
    });
}

export function selectAttachmentFiles(
  files: Iterable<File>,
  existing: readonly Pick<AttachmentDraft, "sizeBytes">[] = [],
): AttachmentSelectionResult {
  const result: AttachmentSelectionResult = {
    countExceeded: false,
    fileTooLarge: false,
    files: [],
    totalTooLarge: false,
  };
  let count = existing.length;
  let totalBytes = existing.reduce((total, attachment) => total + Math.max(0, attachment.sizeBytes), 0);
  for (const file of files) {
    if (file.size <= 0) {
      continue;
    }
    if (file.size > MAX_ATTACHMENT_FILE_BYTES) {
      result.fileTooLarge = true;
      continue;
    }
    if (count >= MAX_ATTACHMENTS_PER_MESSAGE) {
      result.countExceeded = true;
      continue;
    }
    if (totalBytes + file.size > MAX_ATTACHMENT_MESSAGE_BYTES) {
      result.totalTooLarge = true;
      continue;
    }
    result.files.push(file);
    count += 1;
    totalBytes += file.size;
  }
  return result;
}

export function attachmentKindFromMediaType(mediaType: string | null | undefined): AttachmentKind {
  return String(mediaType || "")
    .trim()
    .toLowerCase()
    .startsWith("image/")
    ? "image"
    : "file";
}

export function isImageAttachment(attachment: {
  kind?: string | null;
  mediaType?: string | null;
  media_type?: string | null;
}): boolean {
  const kind = String(attachment.kind || "");
  if (kind === "image") {
    return true;
  }
  const mediaType = String(attachment.mediaType || attachment.media_type || "");
  return attachmentKindFromMediaType(mediaType) === "image";
}

export function formatAttachmentSize(sizeBytes: number | null | undefined): string {
  const size = Math.max(0, Number(sizeBytes || 0));
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KiB`;
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MiB`;
}

function sanitizeDraftIDPart(value: string): string {
  return String(value || "attachment")
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40);
}

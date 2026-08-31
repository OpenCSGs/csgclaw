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

export type AttachmentPreviewItem = {
  downloadURL?: string;
  file?: File;
  id: string;
  mediaType: string;
  name: string;
  previewURL?: string;
  sizeBytes: number;
};

export type AttachmentSelectionResult = {
  countExceeded: boolean;
  duplicateNames: string[];
  fileTooLarge: boolean;
  files: File[];
  totalTooLarge: boolean;
};

export type AttachmentFilenameParts = {
  extension: string;
  stem: string;
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
  existing: readonly (Pick<AttachmentDraft, "sizeBytes"> & Partial<Pick<AttachmentDraft, "name">>)[] = [],
): AttachmentSelectionResult {
  const result: AttachmentSelectionResult = {
    countExceeded: false,
    duplicateNames: [],
    fileTooLarge: false,
    files: [],
    totalTooLarge: false,
  };
  const signatures = new Set(
    existing
      .filter((attachment) => Boolean(attachment.name))
      .map((attachment) => attachmentSignature(attachment.name || "", attachment.sizeBytes)),
  );
  let count = existing.length;
  let totalBytes = existing.reduce((total, attachment) => total + Math.max(0, attachment.sizeBytes), 0);
  for (const file of files) {
    if (file.size <= 0) {
      continue;
    }
    const signature = attachmentSignature(file.name, file.size);
    if (signatures.has(signature)) {
      result.duplicateNames.push(file.name || "attachment");
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
    signatures.add(signature);
    count += 1;
    totalBytes += file.size;
  }
  return result;
}

export function splitAttachmentFilename(name: string): AttachmentFilenameParts {
  const normalized = String(name || "attachment");
  const extensionStart = normalized.lastIndexOf(".");
  if (extensionStart <= 0 || extensionStart === normalized.length - 1) {
    return { extension: "", stem: normalized };
  }
  return {
    extension: normalized.slice(extensionStart),
    stem: normalized.slice(0, extensionStart),
  };
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

function attachmentSignature(name: string, sizeBytes: number): string {
  return `${String(name || "attachment")
    .trim()
    .toLocaleLowerCase()}:${Math.max(0, Number(sizeBytes || 0))}`;
}

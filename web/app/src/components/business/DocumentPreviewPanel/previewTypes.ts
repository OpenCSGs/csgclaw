import type { AttachmentPreviewItem } from "@/models/attachments";
import type { PreviewKind } from "./types";

const mediaKinds = new Map<string, PreviewKind>([
  ["application/pdf", "pdf"],
  ["application/json", "text"],
  ["application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx"],
  ["application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "spreadsheet"],
  ["application/vnd.ms-excel", "spreadsheet"],
  ["application/vnd.openxmlformats-officedocument.presentationml.presentation", "powerpoint"],
  ["text/csv", "text"],
  ["text/markdown", "markdown"],
  ["text/plain", "text"],
]);

const extensionKinds = new Map<string, PreviewKind>([
  ["csv", "text"],
  ["docx", "docx"],
  ["jpeg", "image"],
  ["jpg", "image"],
  ["json", "text"],
  ["md", "markdown"],
  ["markdown", "markdown"],
  ["pdf", "pdf"],
  ["png", "image"],
  ["pptx", "powerpoint"],
  ["svg", "image"],
  ["txt", "text"],
  ["webp", "image"],
  ["xls", "spreadsheet"],
  ["xlsx", "spreadsheet"],
]);

type TextEncoding = "utf-16be" | "utf-16le" | "utf-8";

const textSniffBytes = 64 * 1024;

export const MAX_TEXT_PREVIEW_BYTES = 256 * 1024;

export function documentPreviewKind(
  item: Pick<AttachmentPreviewItem, "mediaType" | "name">,
  data?: ArrayBuffer,
): PreviewKind {
  const mediaType = item.mediaType.toLowerCase().split(";", 1)[0]?.trim() ?? "";
  if (mediaType.startsWith("image/")) {
    return "image";
  }
  const byMediaType = mediaKinds.get(mediaType);
  if (byMediaType) {
    return byMediaType;
  }
  if (mediaType.startsWith("text/")) {
    return "text";
  }
  const extension = item.name.toLowerCase().split(".").pop() ?? "";
  const byExtension = extensionKinds.get(extension);
  if (byExtension) {
    return byExtension;
  }
  return data && detectTextEncoding(data) ? "text" : "unsupported";
}

export function formatPreviewText(data: ArrayBuffer, mediaType: string): string {
  let text: string;
  try {
    text = new TextDecoder(detectTextEncoding(data) ?? "utf-8").decode(data);
  } catch {
    return "";
  }
  if (mediaType.toLowerCase().split(";", 1)[0]?.trim() !== "application/json") {
    return text;
  }
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

function detectTextEncoding(data: ArrayBuffer): TextEncoding | null {
  let bytes: Uint8Array;
  try {
    bytes = new Uint8Array(data);
  } catch {
    return null;
  }
  if (bytes.length === 0) {
    return "utf-8";
  }
  if (bytes.length >= 2 && bytes[0] === 0xff && bytes[1] === 0xfe) {
    return "utf-16le";
  }
  if (bytes.length >= 2 && bytes[0] === 0xfe && bytes[1] === 0xff) {
    return "utf-16be";
  }
  const sampleLength = Math.min(bytes.length, textSniffBytes);
  const sample = bytes.subarray(0, sampleLength);
  if (sample.includes(0)) {
    return null;
  }
  let decoded: string;
  try {
    const decoder = new TextDecoder("utf-8", { fatal: true });
    decoded = decoder.decode(sample, { stream: sampleLength < bytes.length });
  } catch {
    return null;
  }
  let controls = 0;
  let characters = 0;
  for (const character of decoded) {
    const code = character.codePointAt(0) ?? 0;
    characters += 1;
    if ((code < 0x20 && code !== 0x09 && code !== 0x0a && code !== 0x0c && code !== 0x0d) || code === 0x7f) {
      controls += 1;
    }
  }
  return controls <= Math.max(1, Math.floor(characters * 0.01)) ? "utf-8" : null;
}

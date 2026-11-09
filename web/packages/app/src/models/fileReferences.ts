import type { AttachmentPreviewItem } from "./attachments";

export type FileReference =
  | { source: "workspace"; agent_id: string; path: string }
  | { source: "attachment"; attachment_id: string };

export type ComposerFileReference = FileReference & { name: string; label: string };

export function fileReferenceKey(reference: FileReference): string {
  return reference.source === "workspace"
    ? `workspace:${reference.agent_id}:${reference.path}`
    : `attachment:${reference.attachment_id}`;
}

export function referenceForPreview(item: AttachmentPreviewItem): ComposerFileReference | undefined {
  if (item.reference) return item.reference;
  if (item.file || item.id.startsWith("draft-")) return undefined;
  return { source: "attachment", attachment_id: item.id, name: item.name, label: item.name };
}

export function normalizeBrowserURL(value: string): string | null {
  const address = value.trim();
  if (!address || [...address].some((character) => character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127))
    return null;
  if (address.startsWith("/") && !address.startsWith("//")) return null;
  const explicitHTTP = /^https?:\/\//i.test(address);
  // A hostname followed by a port is an address, not an arbitrary URI scheme.
  const hostPort = /^(?:localhost|[^/\s:]+\.[^/\s:]+|\[[0-9a-f:]+\]):\d+(?:[/?#]|$)/i.test(address);
  if (!explicitHTTP && /^[a-z][a-z\d+.-]*:/i.test(address) && !hostPort) return null;
  try {
    const url = new URL(explicitHTTP ? address : `https://${address.replace(/^\/\//, "")}`);
    if (!["https:", "http:"].includes(url.protocol) || url.username || url.password || !url.hostname) return null;
    if (
      !explicitHTTP &&
      (url.hostname === "localhost" ||
        url.hostname.endsWith(".localhost") ||
        url.hostname === "[::1]" ||
        /^127\./.test(url.hostname))
    )
      url.protocol = "http:";
    return url.href;
  } catch {
    return null;
  }
}

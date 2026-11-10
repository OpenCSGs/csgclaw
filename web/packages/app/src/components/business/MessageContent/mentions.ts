export const mentionMarkupPattern = /<at\s+user_id="([^"]+)">([\s\S]*?)<\/at>/g;

export function flattenMentionText(content: unknown): string {
  return String(content ?? "").replace(mentionMarkupPattern, (_, __, name) => `@${name}`);
}

export function escapeHTML(value: unknown): string {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

export const mentionMarkupPattern = /<at\s+user_id="([^"]+)">([\s\S]*?)<\/at>/g;

export function flattenMentionText(content: unknown): string {
  return String(content ?? "").replace(mentionMarkupPattern, (_, __, name) => `@${name}`);
}

export function decorateMentionMarkup(content: unknown): string {
  return String(content ?? "").replace(mentionMarkupPattern, (_, userID, name) => {
    const safeUserID = escapeHTML(userID);
    const safeName = escapeHTML(name);
    return `<span class="message-mention" data-user-id="${safeUserID}">@${safeName}</span>`;
  });
}

export function escapeHTML(value: unknown): string {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

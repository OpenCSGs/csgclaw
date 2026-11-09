import styles from "./MessageMention.module.css";
import { escapeHTML, mentionMarkupPattern } from "./mentions";

export function renderMessageMention(userID: string, name: string): string {
  return `<span class="message-mention ${styles["message-mention"]}" data-user-id="${escapeHTML(userID)}">@${escapeHTML(name)}</span>`;
}

export function decorateMentionMarkup(content: unknown): string {
  return String(content ?? "").replace(mentionMarkupPattern, (_, userID, name) => renderMessageMention(userID, name));
}

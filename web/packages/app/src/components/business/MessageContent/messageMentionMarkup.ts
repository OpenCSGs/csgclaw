import styles from "./MessageMention.module.css";
import type { MessageMention } from "@/models/conversations";
import { escapeHTML, mentionMarkupPattern } from "./mentions";

const mentionClassName = `message-mention ${styles["message-mention"]}`;
const plainMentionPattern =
  /@"(?:[^"\\]|\\.)*"|(?<![\p{L}\p{M}\p{N}._@/\\-])@([\p{L}\p{M}\p{N}._-]+)(?![\p{L}\p{M}\p{N}._/\\-])/gu;

export function renderMessageMention(userID: string, name: string): string {
  return `<span class="${mentionClassName}" data-user-id="${escapeHTML(userID)}">@${escapeHTML(name)}</span>`;
}

export function decorateMentionMarkup(content: unknown): string {
  return String(content ?? "").replace(mentionMarkupPattern, (_, userID, name) => renderMessageMention(userID, name));
}

export function decorateResolvedMentionText(root: DocumentFragment, mentions: readonly MessageMention[]) {
  const usersByName = new Map<string, string | null>();
  for (const mention of mentions) {
    if (typeof mention === "string" || !mention?.id || !mention.name) continue;
    const name = mention.name.trim().toLowerCase();
    const previous = usersByName.get(name);
    usersByName.set(name, previous === undefined || previous === mention.id ? mention.id : null);
  }
  if (!usersByName.size) return;

  const doc = root.ownerDocument;
  const walker = doc.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const candidates: Text[] = [];
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    if (
      node instanceof Text &&
      node.data.includes("@") &&
      !node.parentElement?.closest("a, button, code, pre, .message-mention")
    ) {
      candidates.push(node);
    }
  }
  for (const node of candidates) {
    const fragment = doc.createDocumentFragment();
    let cursor = 0;
    for (const match of node.data.matchAll(plainMentionPattern)) {
      const id = match[1] && usersByName.get(match[1].toLowerCase());
      if (!id) continue;
      const highlight = doc.createElement("span");
      highlight.className = mentionClassName;
      highlight.dataset.userId = id;
      highlight.textContent = match[0];
      fragment.append(node.data.slice(cursor, match.index), highlight);
      cursor = match.index + match[0].length;
    }
    if (!cursor) continue;
    fragment.append(node.data.slice(cursor));
    node.replaceWith(fragment);
  }
}

import type { IMConversation } from "./conversations";

export function taskConversationTitle(message: string, fallback: string): string {
  const text = message.replace(/\s+/g, " ").trim();
  const characters = Array.from(text);
  return characters.length > 32 ? `${characters.slice(0, 32).join("")}…` : text || fallback;
}

export function uniqueTaskConversations(
  pages: readonly { rooms: IMConversation[]; total?: number; has_more?: boolean }[],
): IMConversation[] {
  return Array.from(new Map(pages.flatMap((page) => page.rooms).map((room) => [room.id, room])).values());
}

import { parseTurnProgress, progressActive } from "./turnProgress";
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

export type RefreshScope = "all" | "search";

// Only a replacement of a running projection can bypass server pagination.
// Edits to public text can also change search matches in older messages.
export function taskListChange(
  previous: readonly IMConversation[] | null,
  rooms: readonly IMConversation[],
): RefreshScope | null {
  if (!previous || previous.length !== rooms.length) return "all";
  let scope: RefreshScope | null = null;
  for (const [index, room] of rooms.entries()) {
    const before = previous[index];
    if (before === room) continue;
    const keys = Object.keys({ ...before, ...room }) as (keyof IMConversation)[];
    if (
      keys.some(
        (key) =>
          key !== "messages" && key !== "context_usage" && key !== "context_usage_epoch" && before[key] !== room[key],
      )
    )
      return "all";
    if (before.messages === room.messages) continue;
    if (before.messages.length !== room.messages.length) return "all";
    for (const [index, message] of room.messages.entries()) {
      const prior = before.messages[index];
      if (prior === message) continue;
      if (
        !message.id ||
        prior.id !== message.id ||
        prior.created_at !== message.created_at ||
        prior.thread !== message.thread
      )
        return "all";
      const next = parseTurnProgress(message);
      const old = parseTurnProgress(prior);
      if (!next || !old || next.id !== old.id || !progressActive(next) || !progressActive(old)) return "all";
      if (message.content !== prior.content) scope = "search";
    }
  }
  return scope;
}

// The server owns the preview ID (it can point into a thread). Only overlay a
// newer revision of that same turn while the paginated request is in flight.
export function taskConversationPreview(room: IMConversation, liveRoom?: IMConversation) {
  const stored = room.messages.at(-1);
  const last = liveRoom?.messages.at(-1);
  if (!stored?.id || !last || stored === last) return stored;
  if (stored.id === last.id && stored.content === last.content && stored.created_at === last.created_at) return stored;
  const old = parseTurnProgress(stored);
  if (!old || !progressActive(old)) return stored;
  // Bootstrap can end with a coordination notice excluded by the server list.
  const live = last.id === stored.id ? last : liveRoom?.messages.find((message) => message.id === stored.id);
  const next = parseTurnProgress(live);
  return live && next && old.id === next.id && next.revision > old.revision ? live : stored;
}

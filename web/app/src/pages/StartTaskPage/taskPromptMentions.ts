import { serializeComposerSegments, type ComposerSegment } from "@/models/composer";

export type TaskPromptMention = { start: number; end: number; userId: string; userName: string };

export function updateTaskPromptMentions(before: string, after: string, mentions: readonly TaskPromptMention[]) {
  let start = 0;
  while (start < before.length && start < after.length && before[start] === after[start]) start++;
  let oldEnd = before.length;
  let newEnd = after.length;
  while (oldEnd > start && newEnd > start && before[oldEnd - 1] === after[newEnd - 1]) {
    oldEnd--;
    newEnd--;
  }
  const shift = after.length - before.length;
  return mentions.flatMap((mention) => {
    if (mention.end <= start) return [mention];
    if (mention.start >= oldEnd) return [{ ...mention, start: mention.start + shift, end: mention.end + shift }];
    return [];
  });
}

export function serializeTaskPromptMentions(
  value: string,
  mentions: readonly TaskPromptMention[],
  memberIDs: ReadonlySet<string>,
) {
  const segments: ComposerSegment[] = [];
  let offset = 0;
  for (const mention of [...mentions].sort((a, b) => a.start - b.start)) {
    if (
      !memberIDs.has(mention.userId) ||
      mention.start < offset ||
      value.slice(mention.start, mention.end) !== `@${mention.userName}`
    )
      continue;
    segments.push({ type: "text", text: value.slice(offset, mention.start) });
    segments.push({ type: "mention", userId: mention.userId, userName: mention.userName });
    offset = mention.end;
  }
  segments.push({ type: "text", text: value.slice(offset) });
  return serializeComposerSegments(segments);
}

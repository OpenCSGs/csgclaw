import { composerFileReferences, serializeComposerSegments, type ComposerSegment } from "@/models/composer";

import type { ComposerFileReference } from "@/models/fileReferences";

export type TaskPromptMention = { start: number; end: number } & (
  | { userId: string; userName: string }
  | { reference: ComposerFileReference }
);

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

function taskPromptSegments(value: string, mentions: readonly TaskPromptMention[], memberIDs: ReadonlySet<string>) {
  const segments: ComposerSegment[] = [];
  let offset = 0;
  for (const mention of [...mentions].sort((a, b) => a.start - b.start)) {
    const name = "reference" in mention ? mention.reference.label : mention.userName;
    const text = `@${name}`;
    if (mention.start < offset || value.slice(mention.start, mention.end) !== text) continue;
    if (!("reference" in mention) && !memberIDs.has(mention.userId)) continue;
    segments.push({ type: "text", text: value.slice(offset, mention.start) });
    segments.push(
      "reference" in mention
        ? { type: "file", text, reference: mention.reference }
        : { type: "mention", userId: mention.userId, userName: mention.userName },
    );
    offset = mention.end;
  }
  segments.push({ type: "text", text: value.slice(offset) });
  return segments;
}

export function serializeTaskPromptMentions(
  value: string,
  mentions: readonly TaskPromptMention[],
  memberIDs: ReadonlySet<string>,
) {
  return serializeComposerSegments(taskPromptSegments(value, mentions, memberIDs));
}

export function taskPromptFileReferences(value: string, mentions: readonly TaskPromptMention[]) {
  return composerFileReferences(taskPromptSegments(value, mentions, new Set()));
}

export function withoutTaskPromptFiles(value: string, mentions: readonly TaskPromptMention[]) {
  let text = value;
  let remaining = [...mentions];
  const files = mentions.filter((mention) => "reference" in mention).sort((a, b) => b.start - a.start);
  for (const file of files) {
    const end = text[file.end] === " " ? file.end + 1 : file.end;
    text = text.slice(0, file.start) + text.slice(end);
    remaining = remaining.flatMap((mention) => {
      if (mention === file) return [];
      const shift = mention.start >= end ? file.start - end : 0;
      return shift ? [{ ...mention, start: mention.start + shift, end: mention.end + shift }] : [mention];
    });
  }
  return { text, mentions: remaining };
}

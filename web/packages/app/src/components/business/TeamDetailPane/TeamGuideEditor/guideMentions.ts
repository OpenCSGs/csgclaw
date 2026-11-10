export type GuideMention = { start: number; end: number; query: string };

export function guideMentionAt(value: string, start: number, end = start): GuideMention | null {
  if (start !== end) return null;
  const match = /(?:^|\s)@([^\s@]*)$/u.exec(value.slice(0, start));
  return match ? { start: start - match[1].length - 1, end: start, query: match[1] } : null;
}

export function insertGuideMention(value: string, mention: GuideMention, name: string) {
  const text = `@${name} `;
  return {
    value: value.slice(0, mention.start) + text + value.slice(mention.end),
    caret: mention.start + text.length,
  };
}

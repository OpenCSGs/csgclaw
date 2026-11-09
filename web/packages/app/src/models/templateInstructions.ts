const startMarker = "<!-- BEGIN CSGCLAW-INSTRUCTIONS (auto-generated; do not edit) -->";
const endMarker = "<!-- END CSGCLAW-INSTRUCTIONS -->";
const runtimeHeading = /^# (?:Managed Runtime Instructions|CSGClaw Runtime Boundary|CSGClaw Rules)\b/m;

function customSection(document: string) {
  const start = document.indexOf(startMarker);
  const end = start < 0 ? -1 : document.indexOf(endMarker, start);
  if (start < 0 || end < 0) return null;
  const bodyStart = start + startMarker.length;
  const body = document.slice(bodyStart, end);
  const heading = /^# Agent Instructions\r?\n\r?\n/m.exec(body);
  const from = heading?.index ?? body.search(runtimeHeading);
  const contentStart = heading ? heading.index + heading[0].length : -1;
  const tail = contentStart < 0 ? "" : body.slice(contentStart);
  const next = tail.search(runtimeHeading);
  return {
    bodyStart,
    from: from < 0 ? body.length : from,
    to: heading ? (next < 0 ? body.length : contentStart + next) : from < 0 ? body.length : from,
    content: contentStart < 0 ? "" : (next < 0 ? tail : tail.slice(0, next)).trim(),
  };
}

export function extractManagedAgentInstructions(document: string): string {
  return customSection(document)?.content || "";
}

export function replaceManagedAgentInstructions(document: string, instructions: string): string {
  if (instructions.trim() === extractManagedAgentInstructions(document)) return document;
  const newline = document.includes("\r\n") ? "\r\n" : "\n";
  const custom = instructions.trim()
    ? `# Agent Instructions${newline}${newline}${instructions.trim()}${newline}${newline}`
    : "";
  const section = customSection(document);
  if (section) {
    return (
      document.slice(0, section.bodyStart + section.from) + custom + document.slice(section.bodyStart + section.to)
    );
  }
  return `${document.trimEnd()}${document.trim() ? newline + newline : ""}${startMarker}${newline}${newline}${custom}${endMarker}${newline}`;
}

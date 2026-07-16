export type SlashCommandPayload = {
  arg: string;
  body: string;
  name: string;
};

export type SlashPickerCandidateType = "command" | "skill";

export type SlashSkillOption = {
  description?: string;
  name: string;
};

export type SlashPickerCandidate = {
  description?: string;
  name: string;
  type: SlashPickerCandidateType;
};

const slashCommandNamePattern = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/;
const slashCommandOpenPattern = /^<slash-command(?:\s|>|\/)/;
const slashCommandCloseTag = "</slash-command>";

export function parseSlashCommand(content: unknown): SlashCommandPayload | null {
  const cleaned = String(content ?? "").trim();
  if (!slashCommandOpenPattern.test(cleaned) || typeof DOMParser === "undefined") {
    return null;
  }

  const elementEnd = findSlashCommandElementEnd(cleaned);
  if (elementEnd === null) {
    return null;
  }

  const elementSource = cleaned.slice(0, elementEnd);
  const prompt = cleaned.slice(elementEnd).trim();
  const doc = new DOMParser().parseFromString(elementSource, "application/xml");
  if (doc.querySelector("parsererror")) {
    return null;
  }

  const root = doc.documentElement;
  if (!root || root.localName !== "slash-command" || root.namespaceURI) {
    return null;
  }

  const allowedAttributes = new Set(["name", "arg"]);
  const seenAttributes = new Set<string>();
  for (const attr of Array.from(root.attributes)) {
    if (seenAttributes.has(attr.name)) {
      return null;
    }
    seenAttributes.add(attr.name);
    if (attr.namespaceURI || !allowedAttributes.has(attr.name)) {
      return null;
    }
  }

  const name = (root.getAttribute("name") ?? "").trim();
  const arg = (root.getAttribute("arg") ?? "").trim();
  if (!slashCommandNamePattern.test(name) || /[\r\n\t]/.test(arg) || arg.length > 256) {
    return null;
  }

  for (const child of Array.from(root.childNodes)) {
    if (child.nodeType !== Node.TEXT_NODE && child.nodeType !== Node.CDATA_SECTION_NODE) {
      return null;
    }
  }
  if ((root.textContent ?? "").trim() !== "") {
    return null;
  }

  return {
    arg,
    body: prompt,
    name,
  };
}

export function renderSlashCommandAsText(content: unknown): string | null {
  const command = parseSlashCommand(content);
  if (!command) {
    return null;
  }

  if (command.name === "use-skill") {
    return command.body ? `/${command.arg} ${command.body}` : `/${command.arg}`;
  }
  if (command.name === "new" && (command.arg === "" || command.arg === "conversation")) {
    return command.body ? `/new ${command.body}` : "/new";
  }
  return null;
}

export function renderSlashCommandPreviewText(content: unknown): string {
  const slashCommandText = renderSlashCommandAsText(content);
  if (slashCommandText !== null) {
    return slashCommandText;
  }
  return String(content ?? "");
}

export function isNewConversationSlashCommand(content: unknown): boolean {
  const command = parseSlashCommand(content);
  return Boolean(command?.name === "new" && (command.arg === "" || command.arg === "conversation"));
}

export function skillOptionsFromWorkspace(
  entries: readonly { name?: string; path?: string; type?: string }[],
): SlashSkillOption[] {
  const skillDirs = new Set<string>();
  const dirs = new Set<string>();
  entries.forEach((entry) => {
    const path = String(entry.path || "").trim();
    const parts = path.split("/").filter(Boolean);
    if (parts.length === 1 && entry.type === "dir") {
      dirs.add(path);
    }
    if (parts.length === 2 && parts[1] === "SKILL.md") {
      skillDirs.add(parts[0]);
    }
  });
  return [...dirs]
    .filter((path) => skillDirs.has(path))
    .map((path) => ({ name: path }))
    .filter((skill) => Boolean(skill.name))
    .sort((left, right) => left.name.localeCompare(right.name));
}

export function skillDescriptionFromMarkdown(content: string): string {
  const frontmatterMatch = String(content || "").match(/^---\n([\s\S]*?)\n---/);
  if (!frontmatterMatch) {
    return "";
  }
  const descriptionLine = frontmatterMatch[1].split(/\r?\n/).find((line) => line.trim().startsWith("description:"));
  if (!descriptionLine) {
    return "";
  }
  const description = descriptionLine
    .replace(/^\s*description:\s*/, "")
    .trim()
    .replace(/^['"]|['"]$/g, "");
  return description.slice(0, 220);
}

function findSlashCommandElementEnd(content: string): number | null {
  const openEnd = findTagEndOutsideQuotes(content, 0);
  if (openEnd === null) {
    return null;
  }

  const openTag = content.slice(0, openEnd + 1);
  if (/\/\s*>$/.test(openTag)) {
    return openEnd + 1;
  }

  const closeStart = content.indexOf(slashCommandCloseTag, openEnd + 1);
  if (closeStart < 0) {
    return null;
  }

  const elementBody = content.slice(openEnd + 1, closeStart);
  if (elementBody.trim() !== "") {
    return null;
  }
  return closeStart + slashCommandCloseTag.length;
}

function findTagEndOutsideQuotes(content: string, start: number): number | null {
  let quote: '"' | "'" | null = null;
  for (let idx = start; idx < content.length; idx += 1) {
    const char = content[idx];
    if (quote) {
      if (char === quote) {
        quote = null;
      }
      continue;
    }
    if (char === '"' || char === "'") {
      quote = char;
      continue;
    }
    if (char === ">") {
      return idx;
    }
  }
  return null;
}

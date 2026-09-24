export type SlashCommandPayload = {
  arg: string;
  body: string;
  name: string;
};

export type SlashPickerCandidateType = "command" | "skill";

export type SlashSkillOption = {
  enabled?: boolean;
  description?: string;
  name: string;
};

export type SlashPickerCandidate = {
  description?: string;
  name: string;
  type: SlashPickerCandidateType;
};

const suggestedActionCommands: readonly SlashPickerCandidate[] = [
  {
    description: "建议创建智能体，不会自动执行",
    name: "创建智能体",
    type: "command",
  },
  {
    description: "建议创建房间，不会自动执行",
    name: "创建房间",
    type: "command",
  },
];

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

export function composerActionSuggestions(content: string): SlashPickerCandidate[] {
  const normalized = String(content || "").trim();
  if (!normalized || normalized.startsWith("/")) {
    return [];
  }
  const result: SlashPickerCandidate[] = [];
  if (/(创建|新建).{0,12}(智能体|agent)/iu.test(normalized)) {
    result.push(suggestedActionCommands[0]);
  }
  if (/(创建|新建).{0,12}(房间|room)/iu.test(normalized)) {
    result.push(suggestedActionCommands[1]);
  }
  return result;
}

export function isNewConversationSlashCommand(content: unknown): boolean {
  const command = parseSlashCommand(content);
  return Boolean(command?.name === "new" && (command.arg === "" || command.arg === "conversation"));
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

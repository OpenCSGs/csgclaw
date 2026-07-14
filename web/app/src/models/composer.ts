export type ComposerSegment =
  | {
      text: string;
      type: "text";
    }
  | {
      type: "slash";
      text: string;
    }
  | {
      type: "mention";
      userId: string;
      userName: string;
    };

export type ComposerMentionUser = {
  id: string;
  name?: string | null;
};

export type ComposerMentionState = {
  endOffset: number;
  query: string;
  startOffset: number;
  textNode?: Node;
};

export type ComposerSlashState = {
  endOffset: number;
  query: string;
  startOffset: number;
  textNode: Text;
  tokenElement?: HTMLElement;
};

type ComposerTextQueryContext = {
  isSlashToken?: boolean;
  offset: number;
  slashTokenElement?: HTMLElement;
  textBeforeCursor: string;
  textNode: Text;
};

type ComposerCaretDirection = "backward" | "forward";

const composerCaretAnchorSelector = '[data-composer-caret-anchor="true"]';

type ComposerKeyboardLikeEvent = {
  isComposing?: boolean;
  key?: string;
  keyCode?: number;
  nativeEvent?: {
    isComposing?: boolean;
    keyCode?: number;
    which?: number;
  };
  which?: number;
};

export function createMentionTokenElement(user: ComposerMentionUser): HTMLSpanElement {
  const token = document.createElement("span");
  token.className = "composer-mention-token";
  token.dataset.userId = user.id;
  token.dataset.userName = user.name || user.id;
  token.contentEditable = "false";
  token.textContent = `@${token.dataset.userName}`;
  return token;
}

export function createSlashTokenElement(value: unknown): HTMLSpanElement {
  const token = document.createElement("span");
  token.className = "composer-slash-token";
  token.dataset.composerSlashToken = "true";
  token.contentEditable = "false";
  token.textContent = String(value ?? "");
  return token;
}

const slashTokenPattern = /(^|[\s])\/[A-Za-z0-9._-]+(?!\/)/g;

function splitTextSegmentBySlash(value: unknown): ComposerSegment[] {
  const text = String(value ?? "");
  const segments: ComposerSegment[] = [];
  let last = 0;
  for (const match of text.matchAll(slashTokenPattern)) {
    const fullMatch = match[0] || "";
    const matchText = fullMatch.trimStart();
    const start = (match.index || 0) + (fullMatch.length - matchText.length);
    if (start > last) {
      segments.push({ type: "text", text: text.slice(last, start) });
    }
    segments.push({ type: "slash", text: matchText });
    last = start + matchText.length;
  }
  if (last < text.length) {
    segments.push({ type: "text", text: text.slice(last) });
  }
  return segments;
}

export function normalizeComposerSegmentsForDisplay(
  segments: readonly (ComposerSegment | null | undefined)[] | null | undefined,
): ComposerSegment[] {
  const normalized: ComposerSegment[] = [];
  for (const segment of segments ?? []) {
    if (!segment) {
      continue;
    }
    if (segment.type === "mention") {
      normalized.push({ type: "mention", userId: segment.userId, userName: segment.userName });
      continue;
    }
    const text = String(segment.text ?? "");
    if (!text) {
      continue;
    }
    normalized.push(...splitTextSegmentBySlash(text));
  }
  return normalized;
}

export function appendComposerSegments(
  parent: ParentNode | null | undefined,
  segments: readonly (ComposerSegment | null | undefined)[] | null | undefined,
): void {
  if (!parent) {
    return;
  }
  for (const segment of segments ?? []) {
    if (!segment) {
      continue;
    }
    if (segment.type === "mention") {
      parent.append(
        createMentionTokenElement({
          id: segment.userId,
          name: segment.userName,
        }),
      );
      continue;
    }
    if (segment.type === "slash") {
      parent.append(createSlashTokenElement(segment.text || ""));
      continue;
    }
    const parts = String(segment.text ?? "").split("\n");
    parts.forEach((part, index) => {
      if (part) {
        parent.append(document.createTextNode(part));
      }
      if (index < parts.length - 1) {
        parent.append(document.createElement("br"));
      }
    });
  }
}

export function renderComposerSegments(
  root: HTMLElement | null | undefined,
  segments: readonly (ComposerSegment | null | undefined)[] | null | undefined,
): void {
  if (!root) {
    return;
  }
  root.replaceChildren();
  appendComposerSegments(root, segments);
}

export function parseComposerSegments(root: Node | null | undefined): ComposerSegment[] {
  if (!root) {
    return [];
  }
  const segments: ComposerSegment[] = [];
  collectComposerSegments(root, segments);
  return normalizeComposerSegments(segments);
}

export function collectComposerSegments(node: Node, segments: ComposerSegment[]): void {
  node.childNodes.forEach((child) => {
    if (child.nodeType === Node.TEXT_NODE) {
      segments.push({ type: "text", text: child.textContent ?? "" });
      return;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) {
      return;
    }
    const element = child as HTMLElement;
    if (element.dataset?.userId) {
      segments.push({
        type: "mention",
        userId: element.dataset.userId,
        userName: element.dataset.userName || element.textContent?.replace(/^@/, "") || element.dataset.userId,
      });
      return;
    }
    if (element.dataset?.composerSlashToken) {
      segments.push({ type: "slash", text: element.textContent ?? "" });
      return;
    }
    if (element.dataset?.composerCaretAnchor) {
      return;
    }
    if (element.tagName === "BR") {
      segments.push({ type: "text", text: "\n" });
      return;
    }
    collectComposerSegments(element, segments);
    if (element.tagName === "DIV" || element.tagName === "P") {
      segments.push({ type: "text", text: "\n" });
    }
  });
}

export function normalizeComposerSegments(
  segments: readonly (ComposerSegment | null | undefined)[],
): ComposerSegment[] {
  const normalized: ComposerSegment[] = [];
  for (const segment of segments) {
    if (!segment) {
      continue;
    }
    if (segment.type === "mention") {
      if (!segment.userId) {
        continue;
      }
      normalized.push(segment);
      continue;
    }
    if (segment.type === "slash") {
      normalized.push(segment);
      continue;
    }
    const text = segment.text ?? "";
    if (!text) {
      continue;
    }
    const previous = normalized[normalized.length - 1];
    if (previous?.type === "text") {
      previous.text += text;
    } else {
      normalized.push({ type: "text", text });
    }
  }
  for (;;) {
    const last = normalized[normalized.length - 1];
    if (last?.type !== "text" || !last.text.endsWith("\n")) {
      break;
    }
    last.text = last.text.replace(/\n+$/, "");
    if (!last.text) {
      normalized.pop();
    }
  }
  return normalized;
}

export function segmentsToPlainText(segments: readonly ComposerSegment[] | null | undefined): string {
  return (segments ?? [])
    .map((segment) => {
      if (segment.type === "mention") {
        return `@${segment.userName || segment.userId}`;
      }
      return segment.text ?? "";
    })
    .join("");
}

export function areComposerSegmentsEqual(
  left: readonly ComposerSegment[] | null | undefined,
  right: readonly ComposerSegment[] | null | undefined,
): boolean {
  if (left === right) {
    return true;
  }
  if (!left || !right || left.length !== right.length) {
    return false;
  }
  return left.every((segment, index) => {
    const other = right[index];
    if (!other || segment.type !== other.type) {
      return false;
    }
    if (segment.type === "mention") {
      if (other.type !== "mention") {
        return false;
      }
      return segment.userId === other.userId && segment.userName === other.userName;
    }
    if (other.type === "mention") {
      return false;
    }
    return segment.text === other.text;
  });
}

export function isComposerKeyboardEventComposing(event: ComposerKeyboardLikeEvent | null | undefined): boolean {
  const nativeEvent = event?.nativeEvent;
  return Boolean(
    event?.isComposing ||
    nativeEvent?.isComposing ||
    event?.keyCode === 229 ||
    nativeEvent?.keyCode === 229 ||
    event?.which === 229 ||
    nativeEvent?.which === 229,
  );
}

export function updateDrafts(
  current: Record<string, ComposerSegment[]>,
  conversationID: string,
  segments: readonly ComposerSegment[] | null | undefined,
): Record<string, ComposerSegment[]> {
  const normalized = normalizeComposerSegments(segments ?? []);
  const existing = current[conversationID] ?? [];
  if (areComposerSegmentsEqual(existing, normalized)) {
    return current;
  }
  if (normalized.length === 0) {
    if (!current[conversationID]) {
      return current;
    }
    const next = { ...current };
    delete next[conversationID];
    return next;
  }
  return { ...current, [conversationID]: normalized };
}

export function serializeComposerSegments(segments: readonly ComposerSegment[] | null | undefined): string {
  return (segments ?? [])
    .map((segment) => {
      if (segment.type === "mention") {
        const userID = segment.userId || "";
        const userName = segment.userName || userID;
        return `<at user_id="${userID}">${userName}</at>`;
      }
      return segment.text ?? "";
    })
    .join("");
}

export function splitTextSegmentByMentions(
  text: unknown,
  mentionableUsersByName: Map<string, ComposerMentionUser> | null | undefined,
): ComposerSegment[] {
  const content = String(text ?? "");
  if (!content || !mentionableUsersByName || mentionableUsersByName.size === 0) {
    return content ? [{ type: "text", text: content }] : [];
  }
  const mentionPattern = /(^|[^\p{L}\p{M}\p{N}._-])@([\p{L}\p{M}\p{N}._-]+)/gu;
  const segments: ComposerSegment[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = mentionPattern.exec(content)) !== null) {
    const prefix = match[1] ?? "";
    const name = match[2] ?? "";
    const user = mentionableUsersByName.get(name.toLowerCase());
    if (!user) {
      continue;
    }
    const mentionStart = match.index + prefix.length;
    if (mentionStart > lastIndex) {
      segments.push({ type: "text", text: content.slice(lastIndex, mentionStart) });
    }
    segments.push({
      type: "mention",
      userId: user.id,
      userName: user.name || user.id,
    });
    lastIndex = mentionStart + name.length + 1;
  }
  if (lastIndex < content.length) {
    segments.push({ type: "text", text: content.slice(lastIndex) });
  }
  return segments;
}

export function normalizeTextMentions(
  segments: readonly (ComposerSegment | null | undefined)[] | null | undefined,
  mentionableUsersByName: Map<string, ComposerMentionUser> | null | undefined,
): ComposerSegment[] {
  const normalized: ComposerSegment[] = [];
  for (const segment of segments ?? []) {
    if (!segment) {
      continue;
    }
    if (segment.type === "mention") {
      normalized.push(segment);
      continue;
    }
    normalized.push(...splitTextSegmentByMentions(segment.text ?? "", mentionableUsersByName));
  }
  return normalizeComposerSegments(normalized);
}

export function getMentionCandidates(
  users: readonly (ComposerMentionUser | null | undefined)[] | null | undefined,
  query: unknown,
  options: { limit?: number } = {},
): ComposerMentionUser[] {
  const normalizedQuery = String(query ?? "")
    .trim()
    .toLowerCase();
  const validUsers = (users ?? []).filter((user): user is ComposerMentionUser => Boolean(user?.id));
  if (!normalizedQuery) {
    return validUsers;
  }
  const limit = Number.isFinite(options.limit) ? options.limit : 5;
  return validUsers
    .filter((user) => {
      const name = String(user.name ?? "").toLowerCase();
      return name.includes(normalizedQuery);
    })
    .slice(0, limit);
}

export function getComposerMentionState(root: HTMLElement | null | undefined): ComposerMentionState | null {
  if (!root) {
    return null;
  }
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return null;
  }
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer)) {
    return null;
  }
  const context = getActiveTextQueryContext(range.startContainer, range.startOffset);
  if (!context) {
    return null;
  }
  const match = context.textBeforeCursor.match(/(^|\s)@([a-zA-Z0-9._-]*)$/);
  if (!match) {
    return null;
  }
  const query = match[2] ?? "";
  return {
    query,
    textNode: context.textNode,
    startOffset: context.offset - query.length - 1,
    endOffset: context.offset,
  };
}

export function getActiveTextQueryContext(node: Node, offset: number): ComposerTextQueryContext | null {
  if (node.nodeType === Node.TEXT_NODE) {
    return {
      textNode: node as Text,
      offset,
      textBeforeCursor: (node.textContent ?? "").slice(0, offset),
    };
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return null;
  }
  const child = node.childNodes[offset - 1];
  if (!child || child.nodeType !== Node.TEXT_NODE) {
    return null;
  }
  return {
    textNode: child as Text,
    offset: child.textContent?.length ?? 0,
    textBeforeCursor: child.textContent ?? "",
  };
}

export function getComposerSlashState(root: HTMLElement | null | undefined): ComposerSlashState | null {
  if (!root) {
    return null;
  }
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return null;
  }
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer)) {
    return null;
  }
  let context = getActiveTextQueryContext(range.startContainer, range.startOffset);
  if (!context && range.startContainer.nodeType === Node.ELEMENT_NODE) {
    const tokenContext = getAdjacentSlashTokenContext(range.startContainer, range.startOffset);
    if (tokenContext) {
      context = {
        textNode: tokenContext.textNode,
        offset: tokenContext.textNode.textContent?.length ?? 0,
        textBeforeCursor: tokenContext.textNode.textContent ?? "",
        isSlashToken: true,
        slashTokenElement: tokenContext.element,
      };
    }
  }
  if (!context) {
    return null;
  }
  if (context.isSlashToken) {
    const query = (context.textBeforeCursor ?? "").replace(/^\//, "");
    return {
      query,
      startOffset: 0,
      endOffset: context.offset,
      textNode: context.textNode,
      tokenElement: context.slashTokenElement,
    };
  }
  const match = context.textBeforeCursor.match(/(^|\s)\/([^\s]*)$/);
  if (!match) {
    return null;
  }
  const query = match[2] ?? "";
  return {
    query,
    startOffset: context.offset - query.length - 1,
    endOffset: context.offset,
    textNode: context.textNode,
  };
}

export function replaceMentionQueryWithToken(
  root: HTMLElement | null | undefined,
  mentionState: ComposerMentionState | null | undefined,
  user: ComposerMentionUser | null | undefined,
): boolean {
  if (!root || !mentionState?.textNode || !user) {
    return false;
  }
  const range = document.createRange();
  range.setStart(mentionState.textNode, mentionState.startOffset);
  range.setEnd(mentionState.textNode, mentionState.endOffset);
  range.deleteContents();

  const spacer = document.createTextNode(" ");
  const token = createMentionTokenElement(user);
  const fragment = document.createDocumentFragment();
  fragment.append(token, spacer);
  range.insertNode(fragment);

  const selection = window.getSelection();
  if (!selection) {
    root.focus();
    return true;
  }
  const afterRange = document.createRange();
  afterRange.setStart(spacer, spacer.textContent?.length ?? 0);
  afterRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(afterRange);
  root.focus();
  return true;
}

export function insertComposerLineBreak(root: HTMLElement | null | undefined): void {
  if (!root) {
    return;
  }
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return;
  }
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer)) {
    return;
  }
  range.deleteContents();

  const trailingRange = document.createRange();
  trailingRange.setStart(range.startContainer, range.startOffset);
  trailingRange.setEnd(root, root.childNodes.length);
  const insertsAtEnd = !hasComposerContent(trailingRange.cloneContents());

  root.querySelectorAll(composerCaretAnchorSelector).forEach((anchor) => anchor.remove());

  const br = document.createElement("br");
  range.insertNode(br);

  const nextRange = document.createRange();
  if (insertsAtEnd) {
    const caretAnchor = document.createElement("br");
    caretAnchor.dataset.composerCaretAnchor = "true";
    br.after(caretAnchor);
    nextRange.setStartBefore(caretAnchor);
  } else {
    const spacer = document.createTextNode("");
    br.after(spacer);
    nextRange.setStart(spacer, 0);
  }
  nextRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(nextRange);
}

function hasComposerContent(node: Node): boolean {
  return Array.from(node.childNodes).some((child) => {
    if (child.nodeType === Node.TEXT_NODE) {
      return Boolean(child.textContent);
    }
    if (child.nodeType !== Node.ELEMENT_NODE) {
      return false;
    }
    const element = child as HTMLElement;
    if (element.dataset?.composerCaretAnchor) {
      return false;
    }
    if (element.tagName === "BR" || element.dataset?.userId || element.dataset?.composerSlashToken) {
      return true;
    }
    return hasComposerContent(element);
  });
}

export function insertPlainTextAtSelection(text: string): void {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return;
  }
  const range = selection.getRangeAt(0);
  range.deleteContents();
  const node = document.createTextNode(text);
  range.insertNode(node);
  const nextRange = document.createRange();
  nextRange.setStart(node, text.length);
  nextRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(nextRange);
}

export function insertComposerSegmentsAtSelection(segments: readonly ComposerSegment[] | null | undefined): void {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return;
  }
  const range = selection.getRangeAt(0);
  range.deleteContents();
  const marker = document.createTextNode("");
  const fragment = document.createDocumentFragment();
  appendComposerSegments(fragment, segments);
  fragment.append(marker);
  range.insertNode(fragment);
  const nextRange = document.createRange();
  nextRange.setStart(marker, 0);
  nextRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(nextRange);
}

export function replaceComposerSlashWithSegments(
  root: HTMLElement | null | undefined,
  segments: readonly ComposerSegment[] | null | undefined,
): boolean {
  const slashState = getComposerSlashState(root);
  if (!slashState) {
    return false;
  }

  const range = document.createRange();
  if (slashState.tokenElement) {
    range.selectNode(slashState.tokenElement);
  } else {
    const endOffset = replacementEndsWithWhitespace(segments)
      ? consumeSingleFollowingWhitespace(slashState.textNode, slashState.endOffset)
      : slashState.endOffset;
    range.setStart(slashState.textNode, slashState.startOffset);
    range.setEnd(slashState.textNode, endOffset);
  }
  range.deleteContents();

  const marker = document.createTextNode("");
  const fragment = document.createDocumentFragment();
  appendComposerSegments(fragment, segments);
  fragment.append(marker);
  range.insertNode(fragment);

  const selection = window.getSelection();
  if (!selection) {
    return true;
  }
  const nextRange = document.createRange();
  nextRange.setStart(marker, marker.textContent.length);
  nextRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(nextRange);
  return true;
}

export function getCollapsedSelectionTextOffset(root: Node | null | undefined): number | null {
  if (!root) {
    return null;
  }
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || !selection.isCollapsed) {
    return null;
  }
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer)) {
    return null;
  }
  const prefixRange = range.cloneRange();
  prefixRange.selectNodeContents(root);
  prefixRange.setEnd(range.startContainer, range.startOffset);
  return prefixRange.toString().length;
}

export function removeAdjacentMentionToken(
  root: HTMLElement | null | undefined,
  direction: ComposerCaretDirection,
): boolean {
  if (!root) {
    return false;
  }
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || !selection.isCollapsed) {
    return false;
  }
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer)) {
    return false;
  }
  const token = findAdjacentMentionToken(range.startContainer, range.startOffset, direction);
  if (!token) {
    return false;
  }
  const sibling = direction === "backward" ? token.nextSibling : token.previousSibling;
  token.remove();
  if (sibling?.nodeType === Node.TEXT_NODE && sibling.textContent === " ") {
    sibling.remove();
  }
  placeCaretNearNode(
    root,
    direction === "backward" ? (sibling?.previousSibling ?? root) : (sibling?.nextSibling ?? root),
    direction,
  );
  return true;
}

export function findAdjacentMentionToken(
  node: Node,
  offset: number,
  direction: ComposerCaretDirection,
): HTMLElement | null {
  if (node.nodeType === Node.TEXT_NODE) {
    if (direction === "backward" && offset > 0) {
      return null;
    }
    if (direction === "forward" && offset < (node.textContent?.length ?? 0)) {
      return null;
    }
    const sibling = direction === "backward" ? node.previousSibling : node.nextSibling;
    return isComposerTokenNode(sibling) ? sibling : null;
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return null;
  }
  const index = direction === "backward" ? offset - 1 : offset;
  const sibling = node.childNodes[index];
  return isComposerTokenNode(sibling) ? sibling : null;
}

function isComposerTokenNode(node: Node | null | undefined): node is HTMLElement {
  if (node?.nodeType !== Node.ELEMENT_NODE) {
    return false;
  }
  const element = node as HTMLElement;
  return Boolean(element.dataset?.userId || element.dataset?.composerSlashToken);
}

function getAdjacentSlashTokenContext(node: Node, offset: number): { element: HTMLElement; textNode: Text } | null {
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return null;
  }
  const previous = node.childNodes[offset - 1];
  if (!previous || previous.nodeType !== Node.ELEMENT_NODE) {
    return null;
  }
  const element = previous as HTMLElement;
  if (!element.dataset?.composerSlashToken) {
    return null;
  }
  const textNode = element.firstChild;
  return textNode?.nodeType === Node.TEXT_NODE ? { element, textNode: textNode as Text } : null;
}

function replacementEndsWithWhitespace(segments: readonly ComposerSegment[] | null | undefined): boolean {
  for (let index = (segments?.length ?? 0) - 1; index >= 0; index -= 1) {
    const segment = segments?.[index];
    if (!segment || segment.type === "mention") {
      continue;
    }
    const text = String(segment.text ?? "");
    if (!text) {
      continue;
    }
    return /\s$/.test(text);
  }
  return false;
}

function consumeSingleFollowingWhitespace(textNode: Text, offset: number): number {
  const text = textNode.textContent ?? "";
  return /\s/.test(text.charAt(offset)) ? offset + 1 : offset;
}

export function placeCaretNearNode(
  root: HTMLElement,
  node: Node | null | undefined,
  direction: ComposerCaretDirection,
): void {
  const selection = window.getSelection();
  if (!selection) {
    root.focus();
    return;
  }
  const range = document.createRange();
  if (node?.nodeType === Node.TEXT_NODE) {
    const offset = direction === "backward" ? (node.textContent?.length ?? 0) : 0;
    range.setStart(node, offset);
  } else if (node?.parentNode) {
    const parent = node.parentNode;
    const index = Array.prototype.indexOf.call(parent.childNodes, node);
    range.setStart(parent, direction === "backward" ? index + 1 : index);
  } else {
    range.setStart(root, root.childNodes.length);
  }
  range.collapse(true);
  selection.removeAllRanges();
  selection.addRange(range);
  root.focus();
}

export function placeCaretAtEnd(root: HTMLElement): void {
  placeCaretNearNode(root, root.lastChild, "backward");
}

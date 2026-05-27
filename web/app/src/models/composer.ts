export function createMentionTokenElement(user) {
  const token = document.createElement("span");
  token.className = "composer-mention-token";
  token.dataset.userId = user.id;
  token.dataset.userName = user.name || user.handle || user.id;
  token.contentEditable = "false";
  token.textContent = `@${token.dataset.userName}`;
  return token;
}

export function appendComposerSegments(parent, segments) {
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
          handle: segment.userName,
        }),
      );
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

export function renderComposerSegments(root, segments) {
  if (!root) {
    return;
  }
  root.replaceChildren();
  appendComposerSegments(root, segments);
}

export function parseComposerSegments(root) {
  if (!root) {
    return [];
  }
  const segments = [];
  collectComposerSegments(root, segments);
  return normalizeComposerSegments(segments);
}

export function collectComposerSegments(node, segments) {
  node.childNodes.forEach((child) => {
    if (child.nodeType === Node.TEXT_NODE) {
      segments.push({ type: "text", text: child.textContent ?? "" });
      return;
    }
    if (child.nodeType !== Node.ELEMENT_NODE) {
      return;
    }
    if (child.dataset?.userId) {
      segments.push({
        type: "mention",
        userId: child.dataset.userId,
        userName: child.dataset.userName || child.textContent?.replace(/^@/, "") || child.dataset.userId,
      });
      return;
    }
    if (child.tagName === "BR") {
      segments.push({ type: "text", text: "\n" });
      return;
    }
    collectComposerSegments(child, segments);
    if (child.tagName === "DIV" || child.tagName === "P") {
      segments.push({ type: "text", text: "\n" });
    }
  });
}

export function normalizeComposerSegments(segments) {
  const normalized = [];
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
  while (normalized.at(-1)?.type === "text" && normalized.at(-1).text.endsWith("\n")) {
    normalized.at(-1).text = normalized.at(-1).text.replace(/\n+$/, "");
    if (!normalized.at(-1).text) {
      normalized.pop();
    }
  }
  return normalized;
}

export function segmentsToPlainText(segments) {
  return (segments ?? [])
    .map((segment) => {
      if (segment.type === "mention") {
        return `@${segment.userName || segment.userId}`;
      }
      return segment.text ?? "";
    })
    .join("");
}

export function areComposerSegmentsEqual(left, right) {
  if (left === right) {
    return true;
  }
  if (!left || !right || left.length !== right.length) {
    return false;
  }
  return left.every((segment, index) => {
    const other = right[index];
    return (
      segment.type === other?.type &&
      segment.text === other?.text &&
      segment.userId === other?.userId &&
      segment.userName === other?.userName
    );
  });
}

export function isComposerKeyboardEventComposing(event) {
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

export function updateDrafts(current, conversationID, segments) {
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

export function serializeComposerSegments(segments) {
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

export function splitTextSegmentByMentions(text, mentionableUsersByHandle) {
  const content = String(text ?? "");
  if (!content || !mentionableUsersByHandle || mentionableUsersByHandle.size === 0) {
    return content ? [{ type: "text", text: content }] : [];
  }
  const mentionPattern = /(^|[^\w])@([a-zA-Z0-9._-]+)/g;
  const segments = [];
  let lastIndex = 0;
  let match;
  while ((match = mentionPattern.exec(content)) !== null) {
    const prefix = match[1] ?? "";
    const handle = match[2] ?? "";
    const user = mentionableUsersByHandle.get(handle.toLowerCase());
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
      userName: user.name || user.handle || user.id,
    });
    lastIndex = mentionStart + handle.length + 1;
  }
  if (lastIndex < content.length) {
    segments.push({ type: "text", text: content.slice(lastIndex) });
  }
  return segments;
}

export function normalizeTextMentions(segments, mentionableUsersByHandle) {
  const normalized = [];
  for (const segment of segments ?? []) {
    if (!segment) {
      continue;
    }
    if (segment.type === "mention") {
      normalized.push(segment);
      continue;
    }
    normalized.push(...splitTextSegmentByMentions(segment.text ?? "", mentionableUsersByHandle));
  }
  return normalizeComposerSegments(normalized);
}

export function getMentionCandidates(users, query, options: { limit?: number } = {}) {
  const normalizedQuery = String(query ?? "")
    .trim()
    .toLowerCase();
  const validUsers = (users ?? []).filter((user) => user?.id);
  if (!normalizedQuery) {
    return validUsers;
  }
  const limit = Number.isFinite(options.limit) ? options.limit : 5;
  return validUsers
    .filter((user) => {
      const handle = String(user.handle ?? "").toLowerCase();
      const name = String(user.name ?? "").toLowerCase();
      return handle.includes(normalizedQuery) || name.includes(normalizedQuery);
    })
    .slice(0, limit);
}

export function getComposerMentionState(root) {
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
  return {
    query: match[2],
    textNode: context.textNode,
    startOffset: context.offset - match[2].length - 1,
    endOffset: context.offset,
  };
}

export function getActiveTextQueryContext(node, offset) {
  if (node.nodeType === Node.TEXT_NODE) {
    return {
      textNode: node,
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
    textNode: child,
    offset: child.textContent?.length ?? 0,
    textBeforeCursor: child.textContent ?? "",
  };
}

export function replaceMentionQueryWithToken(root, mentionState, user) {
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
  const afterRange = document.createRange();
  afterRange.setStart(spacer, spacer.textContent.length);
  afterRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(afterRange);
  root.focus();
  return true;
}

export function insertComposerLineBreak(root) {
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
  const br = document.createElement("br");
  const spacer = document.createTextNode("");
  range.insertNode(br);
  br.after(spacer);
  const nextRange = document.createRange();
  nextRange.setStart(spacer, 0);
  nextRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(nextRange);
}

export function insertPlainTextAtSelection(text) {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return;
  }
  const range = selection.getRangeAt(0);
  range.deleteContents();
  const node = document.createTextNode(text);
  range.insertNode(node);
  const nextRange = document.createRange();
  nextRange.setStart(node, node.textContent.length);
  nextRange.collapse(true);
  selection.removeAllRanges();
  selection.addRange(nextRange);
}

export function insertComposerSegmentsAtSelection(segments) {
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

export function removeAdjacentMentionToken(root, direction) {
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

export function findAdjacentMentionToken(node, offset, direction) {
  if (node.nodeType === Node.TEXT_NODE) {
    if (direction === "backward" && offset > 0) {
      return null;
    }
    if (direction === "forward" && offset < (node.textContent?.length ?? 0)) {
      return null;
    }
    const sibling = direction === "backward" ? node.previousSibling : node.nextSibling;
    return sibling?.dataset?.userId ? sibling : null;
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return null;
  }
  const index = direction === "backward" ? offset - 1 : offset;
  const sibling = node.childNodes[index];
  return sibling?.dataset?.userId ? sibling : null;
}

export function placeCaretNearNode(root, node, direction) {
  const selection = window.getSelection();
  const range = document.createRange();
  if (node?.nodeType === Node.TEXT_NODE) {
    const offset = direction === "backward" ? node.textContent.length : 0;
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

export function placeCaretAtEnd(root) {
  placeCaretNearNode(root, root.lastChild, "backward");
}

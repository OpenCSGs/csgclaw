import {
  getComposerMentionState,
  getComposerSlashState,
  getMentionCandidates,
  createSlashTokenElement,
  isComposerKeyboardEventComposing,
  normalizeComposerSegments,
  normalizeTextMentions,
  parseComposerSegments,
  renderComposerSegments,
  replaceComposerSlashWithSegments,
  replaceMentionQueryWithToken,
  segmentsToPlainText,
  serializeComposerSegments,
  splitTextSegmentByMentions,
  updateDrafts,
} from "@/models/composer";

function createComposerRoot() {
  const root = document.createElement("div");
  root.contentEditable = "true";
  document.body.append(root);
  return root;
}

function placeCaret(textNode: Text, offset = textNode.textContent?.length ?? 0) {
  const selection = window.getSelection();
  const range = document.createRange();
  range.setStart(textNode, offset);
  range.collapse(true);
  selection?.removeAllRanges();
  selection?.addRange(range);
}

describe("composer model helpers", () => {
  afterEach(() => {
    document.body.replaceChildren();
    window.getSelection()?.removeAllRanges();
  });

  it("renders and parses text, line breaks, and mention tokens", () => {
    const root = createComposerRoot();
    renderComposerSegments(root, [
      { type: "text", text: "Hello\n" },
      { type: "mention", userId: "u-1", userName: "Alice" },
      { type: "text", text: " welcome" },
    ]);

    expect(root.querySelector("[data-user-id='u-1']")).toHaveTextContent("@Alice");
    expect(parseComposerSegments(root)).toEqual([
      { type: "text", text: "Hello\n" },
      { type: "mention", userId: "u-1", userName: "Alice" },
      { type: "text", text: " welcome" },
    ]);
    expect(segmentsToPlainText(parseComposerSegments(root))).toBe("Hello\n@Alice welcome");
  });

  it("normalizes adjacent text segments and trims trailing line breaks", () => {
    expect(
      normalizeComposerSegments([
        { type: "text", text: "Hello" },
        null,
        { type: "text", text: " world\n\n" },
        { type: "mention", userId: "", userName: "Ignored" },
      ]),
    ).toEqual([{ type: "text", text: "Hello world" }]);
  });

  it("serializes mention segments into server markup", () => {
    expect(
      serializeComposerSegments([
        { type: "text", text: "Ping " },
        { type: "mention", userId: "u-1", userName: "Alice" },
      ]),
    ).toBe('Ping <at user_id="u-1">Alice</at>');
  });

  it("splits plain text mentions using the mentionable user map", () => {
    const users = new Map([["alice", { handle: "alice", id: "u-1", name: "Alice" }]]);

    expect(splitTextSegmentByMentions("Hi @alice and @missing", users)).toEqual([
      { type: "text", text: "Hi " },
      { type: "mention", userId: "u-1", userName: "Alice" },
      { type: "text", text: " and @missing" },
    ]);
    expect(normalizeTextMentions([{ type: "text", text: "Hi @alice" }], users)).toEqual([
      { type: "text", text: "Hi " },
      { type: "mention", userId: "u-1", userName: "Alice" },
    ]);
  });

  it("keeps all room mention candidates visible for an empty mention query", () => {
    const users = [
      { handle: "admin", id: "u-admin", name: "Admin" },
      { handle: "manager", id: "u-manager", name: "manager" },
      { handle: "dev", id: "u-dev", name: "dev" },
      { handle: "ux", id: "u-ux", name: "ux" },
      { handle: "qa", id: "u-qa", name: "qa" },
      { handle: "sales", id: "u-sales", name: "sales" },
    ];

    expect(getMentionCandidates(users, "")).toEqual(users);
    expect(getMentionCandidates(users, "s")).toEqual([users[5]]);
  });

  it("updates draft maps only when normalized segments change", () => {
    const current = { room1: [{ type: "text", text: "Hello" }] };

    expect(updateDrafts(current, "room1", [{ type: "text", text: "Hello" }])).toBe(current);
    expect(updateDrafts(current, "room1", [])).toEqual({});
    expect(updateDrafts(current, "room2", [{ type: "text", text: "Hi" }])).toEqual({
      room1: [{ type: "text", text: "Hello" }],
      room2: [{ type: "text", text: "Hi" }],
    });
  });

  it("detects keyboard events owned by text composition", () => {
    expect(isComposerKeyboardEventComposing({ isComposing: true })).toBe(true);
    expect(isComposerKeyboardEventComposing({ nativeEvent: { isComposing: true } })).toBe(true);
    expect(isComposerKeyboardEventComposing({ keyCode: 229 })).toBe(true);
    expect(isComposerKeyboardEventComposing({ nativeEvent: { which: 229 } })).toBe(true);
    expect(isComposerKeyboardEventComposing({ key: "Enter", keyCode: 13 })).toBe(false);
  });

  it("detects and replaces the active mention query", () => {
    const root = createComposerRoot();
    const text = document.createTextNode("Hello @ali");
    root.append(text);
    placeCaret(text);

    const state = getComposerMentionState(root);
    expect(state).toMatchObject({ query: "ali", startOffset: 6, endOffset: 10 });
    expect(replaceMentionQueryWithToken(root, state, { handle: "alice", id: "u-1", name: "Alice" })).toBe(true);

    expect(parseComposerSegments(root)).toEqual([
      { type: "text", text: "Hello " },
      { type: "mention", userId: "u-1", userName: "Alice" },
      { type: "text", text: " " },
    ]);
  });

  it("replaces the current slash query at caret without resetting the leading content", () => {
    const root = createComposerRoot();
    const text = document.createTextNode("Hi /bas ");
    root.append(text);
    const caret = window.getSelection();
    const range = document.createRange();
    range.setStart(text, 7);
    range.collapse(true);
    caret?.removeAllRanges();
    caret?.addRange(range);

    const state = getComposerSlashState(root);
    expect(state).toMatchObject({ query: "bas", startOffset: 3, endOffset: 7 });
    const replaced = replaceComposerSlashWithSegments(root, [{ type: "text", text: "/basics " }]);

    expect(replaced).toBe(true);
    expect(parseComposerSegments(root)).toEqual([
      { type: "text", text: "Hi /basics " },
    ]);
    const selection = window.getSelection();
    expect(selection?.rangeCount).toBe(1);
    expect(selection?.getRangeAt(0).startContainer.nodeType).toBe(Node.TEXT_NODE);
  });

  it("replaces the current slash query when the caret is placed next to a slash token", () => {
    const root = createComposerRoot();
    root.append(createSlashTokenElement("bas"));
    const range = document.createRange();
    range.setStart(root, 1);
    range.collapse(true);
    const caret = window.getSelection();
    caret?.removeAllRanges();
    caret?.addRange(range);

    const state = getComposerSlashState(root);
    expect(state).toMatchObject({ query: "bas", startOffset: 0, endOffset: 3 });
    const replaced = replaceComposerSlashWithSegments(root, [{ type: "text", text: "/basis " }]);

    expect(replaced).toBe(true);
    expect(parseComposerSegments(root)).toEqual([{ type: "text", text: "/basis " }]);
  });
});

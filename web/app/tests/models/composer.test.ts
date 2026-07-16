import {
  type ComposerSegment,
  getComposerMentionState,
  getComposerSlashQueryAtSelection,
  getComposerSlashState,
  getCollapsedSelectionTextOffset,
  getMentionCandidates,
  createSlashTokenElement,
  insertComposerLineBreak,
  insertPlainTextAtSelection,
  isComposerKeyboardEventComposing,
  normalizeComposerSegments,
  normalizeComposerSegmentsForDisplay,
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

  it("inserts a visible trailing line break with one Shift+Enter action", () => {
    const root = createComposerRoot();
    const text = document.createTextNode("first");
    root.append(text);
    placeCaret(text);

    insertComposerLineBreak(root);

    expect(root.querySelectorAll("br:not([data-composer-caret-anchor])")).toHaveLength(1);
    expect(root.querySelectorAll('[data-composer-caret-anchor="true"]')).toHaveLength(1);
    insertPlainTextAtSelection("second");
    expect(parseComposerSegments(root)).toEqual([{ type: "text", text: "first\nsecond" }]);
  });

  it("keeps repeated trailing line breaks distinct without accumulating caret anchors", () => {
    const root = createComposerRoot();
    const text = document.createTextNode("first");
    root.append(text);
    placeCaret(text);

    insertComposerLineBreak(root);
    insertComposerLineBreak(root);

    expect(root.querySelectorAll("br:not([data-composer-caret-anchor])")).toHaveLength(2);
    expect(root.querySelectorAll('[data-composer-caret-anchor="true"]')).toHaveLength(1);
    insertPlainTextAtSelection("third");
    expect(parseComposerSegments(root)).toEqual([{ type: "text", text: "first\n\nthird" }]);
  });

  it("inserts one line break in the middle of existing text", () => {
    const root = createComposerRoot();
    const text = document.createTextNode("firstsecond");
    root.append(text);
    placeCaret(text, 5);

    insertComposerLineBreak(root);
    insertPlainTextAtSelection("middle");

    expect(root.querySelector("[data-composer-caret-anchor]")).toBeNull();
    expect(parseComposerSegments(root)).toEqual([{ type: "text", text: "first\nmiddlesecond" }]);
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

  it("keeps typed slash queries editable until a picker candidate is applied", () => {
    expect(normalizeComposerSegmentsForDisplay([{ type: "text", text: "/review existing prompt" }])).toEqual([
      { type: "text", text: "/review existing prompt" },
    ]);
    expect(
      normalizeComposerSegmentsForDisplay([
        { type: "slash", text: "/review" },
        { type: "text", text: " existing prompt" },
      ]),
    ).toEqual([
      { type: "slash", text: "/review" },
      { type: "text", text: " existing prompt" },
    ]);
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
    const users = new Map([["alice", { id: "u-1", name: "Alice" }]]);

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
      { id: "u-admin", name: "Admin" },
      { id: "u-manager", name: "manager" },
      { id: "u-dev", name: "dev" },
      { id: "u-ux", name: "ux" },
      { id: "u-qa", name: "qa" },
      { id: "u-sales", name: "sales" },
    ];

    expect(getMentionCandidates(users, "")).toEqual(users);
    expect(getMentionCandidates(users, "s")).toEqual([users[5]]);
  });

  it("updates draft maps only when normalized segments change", () => {
    const current: Record<string, ComposerSegment[]> = { room1: [{ type: "text", text: "Hello" }] };

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
    expect(replaceMentionQueryWithToken(root, state, { id: "u-1", name: "Alice" })).toBe(true);

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
    expect(parseComposerSegments(root)).toEqual([{ type: "text", text: "Hi /basics " }]);
    const selection = window.getSelection();
    expect(selection?.rangeCount).toBe(1);
    expect(selection?.getRangeAt(0).startContainer.nodeType).toBe(Node.TEXT_NODE);
  });

  it("reads a slash query at the caret before trailing draft text", () => {
    const root = createComposerRoot();
    const text = document.createTextNode("/revexisting prompt");
    root.append(text);
    placeCaret(text, 4);

    expect(getComposerSlashQueryAtSelection(root)).toBe("rev");

    text.textContent = "prefix /rev";
    placeCaret(text);
    expect(getComposerSlashQueryAtSelection(root)).toBeNull();
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

  it("reads the collapsed caret offset across slash tokens", () => {
    const root = createComposerRoot();
    renderComposerSegments(root, [
      { type: "slash", text: "/skill" },
      { type: "text", text: " test" },
    ]);

    const range = document.createRange();
    range.setStart(root, root.childNodes.length);
    range.collapse(true);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);

    expect(getCollapsedSelectionTextOffset(root)).toBe("/skill test".length);
  });
});

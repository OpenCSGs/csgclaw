import {
  getComposerMentionState,
  isComposerKeyboardEventComposing,
  normalizeComposerSegments,
  normalizeTextMentions,
  parseComposerSegments,
  renderComposerSegments,
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
});

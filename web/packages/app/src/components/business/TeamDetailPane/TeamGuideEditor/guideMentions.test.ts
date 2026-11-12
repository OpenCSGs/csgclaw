import { guideMentionAt, insertGuideMention } from "./guideMentions";

describe("plain-text guide mentions", () => {
  it.each([
    ["@", { start: 0, end: 1, query: "" }],
    ["Review\n@开发", { start: 7, end: 10, query: "开发" }],
    ["Ask @Dev", { start: 4, end: 8, query: "Dev" }],
    ["mail@example.com", null],
    ["@@Dev", null],
    ["@Developer Herry", null],
    ["@Dev ", null],
  ])("recognizes a mention at the caret in %s", (value, expected) => {
    expect(guideMentionAt(value as string, (value as string).length)).toEqual(expected);
  });

  it("does not replace selected text and preserves content after the caret", () => {
    const value = "Ask @De to review";
    expect(guideMentionAt(value, 5, 7)).toBeNull();
    const mention = guideMentionAt(value, 7);
    expect(mention).toEqual({ start: 4, end: 7, query: "De" });
    expect(insertGuideMention(value, mention!, "开发工程师 Herry")).toEqual({
      value: "Ask @开发工程师 Herry  to review",
      caret: 17,
    });
  });
});

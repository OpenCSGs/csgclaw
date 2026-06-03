import {
  normalizeSlashShorthandForPayload,
  slashSkillCommandText,
  slashSkillInputText,
  slashSkillQueryForDraft,
} from "@/hooks/workspace/useConversationController";

describe("useConversationController slash skill helpers", () => {
  it("keeps the skill picker open only while editing the command name", () => {
    expect(slashSkillQueryForDraft("/")).toBe("");
    expect(slashSkillQueryForDraft("  /sk")).toBe("sk");
    expect(slashSkillQueryForDraft("/skill-creator ")).toBeNull();
    expect(slashSkillQueryForDraft("/skill-creator make a skill")).toBeNull();
    expect(slashSkillQueryForDraft("hello /skill-creator")).toBeNull();
  });

  it("renders selected skills as canonical slash-command XML", () => {
    expect(slashSkillCommandText("skill-creator")).toBe(
      '<slash-command name="use-skill" arg="skill-creator"></slash-command>',
    );
    expect(slashSkillCommandText('a&b"c<d>')).toBe(
      '<slash-command name="use-skill" arg="a&amp;b&quot;c&lt;d&gt;"></slash-command>',
    );
  });

  it("renders skill input as /slug command text", () => {
    expect(slashSkillInputText("skill-creator")).toBe("/skill-creator ");
    expect(slashSkillInputText(" manager-worker-dispatch ")).toBe("/manager-worker-dispatch ");
  });

  it("normalizes slash-command shorthand into canonical XML before send", () => {
    expect(normalizeSlashShorthandForPayload("/skill-creator")).toBe(
      '<slash-command name="use-skill" arg="skill-creator"></slash-command>',
    );
    expect(normalizeSlashShorthandForPayload("/skill-creator build a review")).toBe(
      '<slash-command name="use-skill" arg="skill-creator"></slash-command> build a review',
    );
    expect(normalizeSlashShorthandForPayload("  /skill-creator   build a review  ")).toBe(
      '<slash-command name="use-skill" arg="skill-creator"></slash-command> build a review',
    );
  });

  it("does not normalize invalid shorthand payloads", () => {
    expect(normalizeSlashShorthandForPayload("/skill/creator")).toBe("/skill/creator");
    expect(normalizeSlashShorthandForPayload("/")).toBe("/");
    expect(normalizeSlashShorthandForPayload("hello /skill-creator")).toBe("hello /skill-creator");
    expect(normalizeSlashShorthandForPayload("//skill-creator")).toBe("//skill-creator");
  });
});

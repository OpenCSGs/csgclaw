import { describe, expect, it } from "vitest";
import { messageTokenUsage, tokenCountLabel, visibleUsageModel } from "@/models/tokenUsage";

describe("turn token usage", () => {
  const usage = { known: true, model_id: "auto", input_tokens: 1000, output_tokens: 220, total_tokens: 1220 };
  const metadata = (value: unknown, status = "succeeded") => ({ csgclaw: { turn_progress: { status, usage: value } } });
  it("reads persisted consumption and preserves a real zero", () => {
    expect(messageTokenUsage(metadata(usage))).toMatchObject(usage);
    expect(messageTokenUsage(metadata({ ...usage, input_tokens: 0, output_tokens: 0, total_tokens: 0 }))).toMatchObject(
      { known: true, total_tokens: 0 },
    );
    expect(messageTokenUsage(metadata({ ...usage, known: false }))).toMatchObject({ known: false });
    expect(messageTokenUsage(metadata(usage, "interrupted"))).not.toBeNull();
  });
  it("rejects malformed or running usage", () => {
    for (const value of [
      null,
      {},
      { ...usage, total_tokens: 50 },
      { ...usage, input_tokens: -1 },
      { ...usage, output_tokens: 1.5 },
      { ...usage, cached_input_tokens: 2000 },
    ]) {
      expect(messageTokenUsage(metadata(value))).toBeNull();
    }
    expect(messageTokenUsage(metadata(usage, "running"))).toBeNull();
    expect(messageTokenUsage({})).toBeNull();
  });
  it("formats counts and hides only the selected auto model", () => {
    expect([0, 999, 1220, 1000000].map(tokenCountLabel)).toEqual(["0", "999", "1.22 K", "1 M"]);
    expect(visibleUsageModel(" AUTO ")).toBe("");
    expect(visibleUsageModel("gpt-test")).toBe("gpt-test");
  });
});

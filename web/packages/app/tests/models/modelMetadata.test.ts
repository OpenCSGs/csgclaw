import { describe, expect, it } from "vitest";
import {
  contextInputValue,
  contextUnit,
  formatContextSize,
  parseContextSize,
  DEFAULT_CONTEXT_TOKENS,
} from "@/models/modelMetadata";
describe("context display units", () => {
  it("uses decimal K tokens and switches to M tokens at one million tokens", () => {
    expect(DEFAULT_CONTEXT_TOKENS).toBe(200000);
    expect(formatContextSize(200000)).toBe("200 K tokens");
    expect(formatContextSize(1000000)).toBe("1 M tokens");
    expect(formatContextSize(1050000)).toBe("1.05 M tokens");
  });
  it("round trips exact token counts without display rounding", () => {
    for (const n of [1, 8192, 200000, 262144, 999999, 1000000, 1048576, 1050000, 1000000000])
      expect(parseContextSize(contextInputValue(n), contextUnit(n))).toBe(n);
    expect(parseContextSize("200", "K")).toBe(200000);
    expect(parseContextSize("1.05", "M")).toBe(1050000);
  });
  it("rejects invalid, fractional-token and excessive sizes", () => {
    for (const value of ["", "-1", "zero", "1e3", "0.0001", "1000001"])
      expect(parseContextSize(value, "K")).toBeUndefined();
  });
});

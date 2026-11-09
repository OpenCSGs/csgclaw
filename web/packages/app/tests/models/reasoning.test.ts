import { describe, expect, it } from "vitest";
import { normalizeReasoningEffort, reasoningSliderIndex } from "@/models/reasoning";

describe("reasoning profile contract", () => {
  it("uses none as the common disabled value and accepts the Codex alias", () => {
    expect(normalizeReasoningEffort(" off ")).toBe("none");
    expect(normalizeReasoningEffort("none")).toBe("none");
  });

  it("uses model default when the profile has no explicit effort", () => {
    expect(normalizeReasoningEffort("")).toBe("auto");
    expect(normalizeReasoningEffort(undefined)).toBe("auto");
    expect(normalizeReasoningEffort("minimal")).toBe("minimal");
  });

  it.each([
    ["none", 0],
    [" off ", 0],
    ["minimal", 1],
    ["low", 2],
    [" MEDIUM ", 3],
    ["high", 4],
    ["xhigh", 5],
    ["auto", null],
    ["", null],
    [undefined, null],
    ["unknown", null],
  ])("maps %s to an explicit slider step or leaves it automatic", (value, expected) => {
    expect(reasoningSliderIndex(value)).toBe(expected);
  });
});

it("uses only advertised levels, preserving explicit empty capabilities", async () => {
  const { reasoningSteps, normalizeReasoningEfforts } = await import("@/models/reasoning");
  const supported = ["none", "low", "medium", "high", "xhigh", "max"];
  expect(reasoningSteps(supported)).toEqual(supported);
  expect(reasoningSteps([])).toEqual([]);
  expect(normalizeReasoningEfforts({ unknown: [] })).toEqual({ unknown: [] });
  expect(normalizeReasoningEfforts({ "gpt-6-luna": supported, bad: ["invalid"] })).toEqual({
    "gpt-6-luna": supported,
    bad: [],
  });
});

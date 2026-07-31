import { describe, expect, it } from "vitest";
import { normalizeReasoningEffort } from "@/models/reasoning";

describe("reasoning profile contract", () => {
  it("uses none as the common disabled value and accepts the OpenClaw alias", () => {
    expect(normalizeReasoningEffort(" off ")).toBe("none");
    expect(normalizeReasoningEffort("none")).toBe("none");
  });

  it("uses model default when the profile has no explicit effort", () => {
    expect(normalizeReasoningEffort("")).toBe("auto");
    expect(normalizeReasoningEffort(undefined)).toBe("auto");
    expect(normalizeReasoningEffort("minimal")).toBe("minimal");
  });
});

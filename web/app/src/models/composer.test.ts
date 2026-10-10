import { describe, expect, it } from "vitest";
import {
  createResourceTokenElement,
  parseComposerSegments,
  serializeComposerSegments,
  type ComposerSegment,
} from "./composer";

describe("composer resource tokens", () => {
  it("renders as a badge while preserving the serialized command text", () => {
    const root = document.createElement("div");
    root.append(
      createResourceTokenElement({
        displayText: "/reviewer",
        kind: "skill",
        name: "reviewer",
        text: "/skill:reviewer",
      }),
      document.createTextNode(" please check this"),
    );

    const segments = parseComposerSegments(root) as ComposerSegment[];
    expect(segments).toEqual([
      {
        type: "resource",
        displayText: "/reviewer",
        kind: "skill",
        name: "reviewer",
        text: "/skill:reviewer",
      },
      { type: "text", text: " please check this" },
    ]);
    expect(serializeComposerSegments(segments)).toBe("/skill:reviewer please check this");
  });
});

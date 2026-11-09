import { describe, expect, it } from "vitest";
import {
  clampPrimarySidebarWidth,
  workspacePrimarySidebarWidthBounds,
  workspaceSidebarWidthBounds,
} from "./sidebarDimensions";

describe("workspace sidebar dimensions", () => {
  it("keeps the primary navigation usable at its minimum width", () => {
    expect(workspacePrimarySidebarWidthBounds()).toMatchObject({ min: 200 });
    expect(clampPrimarySidebarWidth(120)).toBe(200);
  });

  it("reserves a 200px minimum for the context sidebar", () => {
    expect(workspaceSidebarWidthBounds(200)).toMatchObject({ min: 400 });
    expect(workspaceSidebarWidthBounds(240)).toMatchObject({ min: 440 });
    expect(workspaceSidebarWidthBounds(300)).toMatchObject({ min: 500 });
  });
});

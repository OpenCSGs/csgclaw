import { classNames } from "@/shared/lib/classNames";
import { toggleSelection } from "@/shared/lib/collections";

describe("shared library helpers", () => {
  it("joins truthy CSS class names", () => {
    expect(classNames("button", false, null, undefined, "active")).toBe("button active");
  });

  it("toggles selected ids without mutating the original selection", () => {
    const selected = ["a", "b"];
    expect(toggleSelection(selected, "c")).toEqual(["a", "b", "c"]);
    expect(toggleSelection(selected, "a")).toEqual(["b"]);
    expect(selected).toEqual(["a", "b"]);
  });
});

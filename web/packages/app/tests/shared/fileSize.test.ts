import { describe, expect, it } from "vitest";
import { formatFileSize } from "@/shared/lib/fileSize";

describe("human-readable file sizes", () => {
  it.each([
    [0, "0 B"],
    [999, "999 B"],
    [1000, "1 KB"],
    [23882, "23.9 KB"],
    [999949, "999.9 KB"],
    [999950, "1 MB"],
    [1000000, "1 MB"],
    [2500000000, "2.5 GB"],
    [10 ** 12, "1 TB"],
    [10 ** 15, "1 PB"],
  ])("formats %s bytes as %s", (bytes, expected) => {
    expect(formatFileSize(bytes)).toBe(expected);
  });

  it.each([null, undefined, -1, Number.NaN, Number.POSITIVE_INFINITY])(
    "handles invalid or missing size %s",
    (bytes) => {
      expect(formatFileSize(bytes)).toBe("0 B");
    },
  );
});

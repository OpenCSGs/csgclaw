import { describe, expect, it } from "vitest";
import { dataTransferHasFiles, filesFromDataTransfer } from "@/components/business/ConversationPane/attachmentFiles";

describe("attachment drag data", () => {
  it.each([
    ["standard Files type", { types: ["Files"], items: [] }],
    ["case-insensitive Files type", { types: ["files"], items: [] }],
    ["Firefox file type", { types: ["application/x-moz-file"], items: [] }],
    ["file item fallback", { types: [], items: [{ kind: "file" }] }],
  ])("recognizes %s during dragover", (_name, value) => {
    expect(dataTransferHasFiles(value as unknown as DataTransfer)).toBe(true);
  });

  it("ignores text-only drag data", () => {
    expect(
      dataTransferHasFiles({
        types: ["text/plain"],
        items: [{ kind: "string" }],
      } as unknown as DataTransfer),
    ).toBe(false);
  });

  it("reads non-empty files only when the drop data is available", () => {
    const file = new File(["report"], "report.txt", { type: "text/plain" });
    const emptyFile = new File([], "empty.txt", { type: "text/plain" });

    expect(filesFromDataTransfer({ files: [file, emptyFile], items: [] } as unknown as DataTransfer)).toEqual([file]);
  });
});

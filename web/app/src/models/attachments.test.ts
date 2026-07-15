import { describe, expect, it } from "vitest";
import {
  createAttachmentDrafts,
  formatAttachmentSize,
  isImageAttachment,
  MAX_ATTACHMENT_FILE_BYTES,
  MAX_ATTACHMENT_MESSAGE_BYTES,
  MAX_ATTACHMENTS_PER_MESSAGE,
  selectAttachmentFiles,
} from "@/models/attachments";

describe("attachment drafts", () => {
  it("classifies image files and keeps stable display metadata", () => {
    const [draft] = createAttachmentDrafts([new File(["image"], "diagram.png", { type: "image/png" })]);

    expect(draft).toMatchObject({
      name: "diagram.png",
      kind: "image",
      mediaType: "image/png",
      sizeBytes: 5,
    });
    expect(draft.id).toMatch(/^draft-/);
    expect(isImageAttachment(draft)).toBe(true);
  });

  it("formats compact file sizes", () => {
    expect(formatAttachmentSize(999)).toBe("999 B");
    expect(formatAttachmentSize(1536)).toBe("1.5 KiB");
    expect(formatAttachmentSize(2 * 1024 * 1024)).toBe("2.0 MiB");
  });

  it("enforces count, per-file, and total selection limits", () => {
    const oversized = fileWithSize("large.bin", MAX_ATTACHMENT_FILE_BYTES + 1);
    expect(selectAttachmentFiles([oversized])).toMatchObject({ files: [], fileTooLarge: true });

    const countLimited = selectAttachmentFiles(
      [fileWithSize("extra.txt", 1)],
      Array.from({ length: MAX_ATTACHMENTS_PER_MESSAGE }, () => ({ sizeBytes: 1 })),
    );
    expect(countLimited).toMatchObject({ files: [], countExceeded: true });

    const totalLimited = selectAttachmentFiles(
      [fileWithSize("extra.txt", 2)],
      [{ sizeBytes: MAX_ATTACHMENT_MESSAGE_BYTES - 1 }],
    );
    expect(totalLimited).toMatchObject({ files: [], totalTooLarge: true });
  });
});

function fileWithSize(name: string, size: number): File {
  return { name, size, type: "application/octet-stream" } as File;
}

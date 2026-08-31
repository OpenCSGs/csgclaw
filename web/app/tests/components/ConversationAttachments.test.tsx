import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { MessageAttachments } from "@/components/business/ConversationPane/ConversationAttachments";
import type { MessageAttachment } from "@/models/attachments";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key, params) => {
  const labels: Record<string, string> = {
    attachment: "Attachment",
    previewAttachmentNamed: `Preview attachment: ${params?.name ?? ""}`,
  };
  return labels[key] ?? key;
};

describe("MessageAttachments", () => {
  it("opens all capability-backed attachments through the application base path", async () => {
    const user = userEvent.setup();
    const onPreviewAttachment = vi.fn();
    const base = document.createElement("base");
    base.href = "/v1/sandboxes/csgship-test/";
    document.head.prepend(base);
    const attachments: MessageAttachment[] = [
      {
        id: "att-image",
        name: "diagram.png",
        kind: "image",
        media_type: "image/png",
        size_bytes: 42,
        sha256: "image-sha",
        created_at: "2026-07-10T00:00:00Z",
        download_url: "/api/v1/attachments/att-image?token=image-token",
        preview_url: "/api/v1/attachments/att-image?token=image-token",
      },
      {
        id: "att-file",
        name: "report.md",
        kind: "file",
        media_type: "text/markdown",
        size_bytes: 128,
        sha256: "file-sha",
        created_at: "2026-07-10T00:00:00Z",
        download_url: "/api/v1/attachments/att-file?token=file-token",
      },
    ];

    try {
      render(<MessageAttachments attachments={attachments} t={t} onPreviewAttachment={onPreviewAttachment} />);

      const image = screen.getByRole("img", { name: "diagram.png" }) as HTMLImageElement;
      expect(image).toHaveAttribute("src", "api/v1/attachments/att-image?token=image-token");
      expect(new URL(image.src).pathname).toBe("/v1/sandboxes/csgship-test/api/v1/attachments/att-image");
      expect(image).toHaveAttribute("loading", "lazy");

      await user.click(screen.getByRole("button", { name: "Preview attachment: report.md" }));
      expect(onPreviewAttachment).toHaveBeenCalledTimes(1);
      const request = onPreviewAttachment.mock.calls[0]?.[0];
      expect(request.index).toBe(1);
      expect(request.items).toEqual([
        expect.objectContaining({
          id: "att-image",
          previewURL: "api/v1/attachments/att-image?token=image-token",
        }),
        expect.objectContaining({
          id: "att-file",
          downloadURL: "api/v1/attachments/att-file?token=file-token",
          mediaType: "text/markdown",
        }),
      ]);
      expect(request.anchor).toBeInstanceOf(HTMLButtonElement);
    } finally {
      base.remove();
    }
  });
});

import { render, screen } from "@testing-library/react";
import { MessageAttachments } from "@/components/business/ConversationPane/ConversationAttachments";
import type { MessageAttachment } from "@/models/attachments";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key) => (key === "attachment" ? "Attachment" : key);

describe("MessageAttachments", () => {
  it("renders capability-backed attachments relative to the application base path", () => {
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
        name: "report.txt",
        kind: "file",
        media_type: "text/plain",
        size_bytes: 128,
        sha256: "file-sha",
        created_at: "2026-07-10T00:00:00Z",
        download_url: "/api/v1/attachments/att-file?token=file-token",
      },
    ];

    try {
      render(<MessageAttachments attachments={attachments} t={t} />);

      const image = screen.getByRole("img", { name: "diagram.png" }) as HTMLImageElement;
      expect(image).toHaveAttribute("src", "api/v1/attachments/att-image?token=image-token");
      expect(new URL(image.src).pathname).toBe("/v1/sandboxes/csgship-test/api/v1/attachments/att-image");
      expect(image).toHaveAttribute("loading", "lazy");
      expect(image).toHaveAttribute("referrerpolicy", "no-referrer");

      const file = screen.getByRole("link", { name: /report\.txt/ }) as HTMLAnchorElement;
      expect(file).toHaveAttribute("href", "api/v1/attachments/att-file?token=file-token");
      expect(new URL(file.href).pathname).toBe("/v1/sandboxes/csgship-test/api/v1/attachments/att-file");
      expect(file).toHaveAttribute("download");
    } finally {
      base.remove();
    }
  });
});

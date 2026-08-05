import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { MessageAttachments } from "@/components/business/ConversationPane/ConversationAttachments";
import type { MessageAttachment } from "@/models/attachments";
import type { TranslateFn } from "@/models/conversations";

const t: TranslateFn = (key, params) => {
  const labels: Record<string, string> = {
    attachment: "Attachment",
    attachmentPreviewDescription: "Preview without leaving the app",
    attachmentPreviewFailed: "Preview failed",
    attachmentPreviewLoading: "Loading preview",
    attachmentPreviewUnavailable: "Preview unavailable",
    close: "Close",
    downloadAttachment: "download",
    previewAttachmentNamed: `Preview attachment: ${params?.name ?? ""}`,
  };
  return labels[key] ?? key;
};

describe("MessageAttachments", () => {
  it("previews capability-backed attachments in-app relative to the application base path", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn<typeof fetch>(
      async () => new Response("report", { headers: { "Content-Type": "text/plain" }, status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);
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
      {
        id: "att-unsupported",
        name: "report.docx",
        kind: "file",
        media_type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        size_bytes: 256,
        sha256: "unsupported-sha",
        created_at: "2026-07-10T00:00:00Z",
        download_url: "/api/v1/attachments/att-unsupported?token=unsupported-token",
      },
    ];

    try {
      render(<MessageAttachments attachments={attachments} t={t} />);

      const image = screen.getByRole("img", { name: "diagram.png" }) as HTMLImageElement;
      expect(image).toHaveAttribute("src", "api/v1/attachments/att-image?token=image-token");
      expect(new URL(image.src).pathname).toBe("/v1/sandboxes/csgship-test/api/v1/attachments/att-image");
      expect(image).toHaveAttribute("loading", "lazy");
      expect(image).toHaveAttribute("referrerpolicy", "no-referrer");

      await user.click(screen.getByRole("button", { name: "Preview attachment: diagram.png" }));
      expect(screen.getByRole("dialog", { name: "diagram.png" })).toBeInTheDocument();
      const download = screen.getByRole("link", { name: "download" }) as HTMLAnchorElement;
      expect(new URL(download.href).pathname).toBe("/v1/sandboxes/csgship-test/api/v1/attachments/att-image");
      await user.click(screen.getByRole("button", { name: "Close" }));

      await user.click(screen.getByRole("button", { name: "Preview attachment: report.txt" }));
      expect(screen.getByRole("dialog", { name: "report.txt" })).toBeInTheDocument();
      const frame = (await screen.findByTitle("Preview attachment: report.txt")) as HTMLIFrameElement;
      expect(frame.src).toMatch(/^blob:/);
      expect(fetchMock).toHaveBeenCalledWith("api/v1/attachments/att-file?token=file-token", {
        credentials: "same-origin",
        signal: expect.any(AbortSignal),
      });
      await user.click(screen.getByRole("button", { name: "Close" }));

      await user.click(screen.getByRole("button", { name: "Preview attachment: report.docx" }));
      expect(screen.getByRole("dialog", { name: "report.docx" })).toBeInTheDocument();
      expect(screen.getByText("Preview unavailable")).toBeInTheDocument();
      expect(fetchMock).toHaveBeenCalledTimes(1);
      const unsupportedDownload = screen.getByRole("link", { name: "download" }) as HTMLAnchorElement;
      expect(new URL(unsupportedDownload.href).pathname).toBe(
        "/v1/sandboxes/csgship-test/api/v1/attachments/att-unsupported",
      );
    } finally {
      base.remove();
      vi.unstubAllGlobals();
    }
  });
});

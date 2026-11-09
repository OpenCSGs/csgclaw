import { resolveImageAttachmentURL } from "@/components/business/MessageContent/imageAttachments";
import { renderMarkdownWithCitations } from "@/components/business/MessageContent/markdown";
import type { MessageAttachment } from "@/models/attachments";

const attachment: MessageAttachment = {
  id: "qr-image",
  name: "feishu-im-auth-qrcode.png",
  kind: "image",
  media_type: "image/png",
  size_bytes: 42,
  sha256: "image-sha",
  created_at: "2026-11-09T00:00:00Z",
  workspace_path: ".csgclaw/attachments/qr-image-feishu-im-auth-qrcode.png",
  download_url: "/api/v1/attachments/qr-image?token=image-token",
};

describe("inline image attachments", () => {
  it.each([
    "feishu-im-auth-qrcode.png",
    "./feishu-im-auth-qrcode.png",
    "images/feishu-im-auth-qrcode.png",
    "/workspace/feishu-im-auth-qrcode.png",
    "C:\\workspace\\feishu-im-auth-qrcode.png",
    "feishu-im-auth-qrcode.png?version=1#preview",
    "./.csgclaw/attachments/qr-image-feishu-im-auth-qrcode.png",
  ])("resolves the delivered image for %s", (source) => {
    expect(resolveImageAttachmentURL(source, [attachment])).toBe("api/v1/attachments/qr-image?token=image-token");
  });

  it("matches percent-encoded Unicode names and spaces", () => {
    const image = { ...attachment, name: "飞书 授权二维码.png" };
    expect(resolveImageAttachmentURL(`images/${encodeURIComponent(image.name)}`, [image])).toBe(
      "api/v1/attachments/qr-image?token=image-token",
    );
  });

  it("prefers an exact workspace path when image names collide", () => {
    const first = { ...attachment, workspace_path: "first/feishu-im-auth-qrcode.png" };
    const second = {
      ...attachment,
      id: "qr-other",
      workspace_path: "second/feishu-im-auth-qrcode.png",
      preview_url: "/api/v1/attachments/qr-other?token=other-token",
    };
    expect(resolveImageAttachmentURL("./second/feishu-im-auth-qrcode.png", [first, second])).toBe(
      "api/v1/attachments/qr-other?token=other-token",
    );
    expect(resolveImageAttachmentURL(attachment.name, [first, second])).toBeUndefined();
    expect(
      resolveImageAttachmentURL(first.workspace_path, [first, { ...second, workspace_path: first.workspace_path }]),
    ).toBeUndefined();
  });

  it.each([
    "https://example.com/feishu-im-auth-qrcode.png",
    "//example.com/feishu-im-auth-qrcode.png",
    "data:image/png;base64,abc",
    "blob:https://example.com/feishu-im-auth-qrcode.png",
    "file:///workspace/feishu-im-auth-qrcode.png",
    "javascript:feishu-im-auth-qrcode.png",
    "missing.png",
    "bad%ZZ.png",
    "",
  ])("leaves unmatched or URL references unchanged: %s", (source) => {
    expect(resolveImageAttachmentURL(source, [attachment])).toBeUndefined();
  });

  it("requires an image attachment with a usable URL", () => {
    expect(resolveImageAttachmentURL(attachment.name, [])).toBeUndefined();
    expect(
      resolveImageAttachmentURL(attachment.name, [{ ...attachment, kind: "file", media_type: "text/plain" }]),
    ).toBeUndefined();
    expect(resolveImageAttachmentURL(attachment.name, [{ ...attachment, download_url: "" }])).toBeUndefined();
    expect(resolveImageAttachmentURL(attachment.name, [{ ...attachment, kind: "file" }])).toBe(
      "api/v1/attachments/qr-image?token=image-token",
    );
  });

  it("renders local Markdown and HTML images alongside citations without changing code or links", () => {
    const content = [
      `![飞书授权二维码](${attachment.name})`,
      `<img src="${attachment.name}" alt="HTML image" onerror="alert(1)">`,
      `\`![example](${attachment.name})\``,
      `[download](${attachment.name})`,
      '([Guide][1])\n\n[1]: "A cited passage"',
    ].join("\n\n");
    const rendered = renderMarkdownWithCitations(content, undefined, [attachment]);
    const template = document.createElement("template");
    template.innerHTML = rendered.html;

    const images = [...template.content.querySelectorAll("img")];
    expect(images).toHaveLength(2);
    for (const image of images) {
      expect(image.getAttribute("src")).toBe("api/v1/attachments/qr-image?token=image-token");
      expect(image.hasAttribute("onerror")).toBe(false);
    }
    expect(template.content.querySelector("code")?.textContent).toBe(`![example](${attachment.name})`);
    expect(template.content.querySelector("a")?.getAttribute("href")).toBe(attachment.name);
    expect(rendered.cited).toHaveLength(1);
    expect(template.content.querySelector("button")?.getAttribute("data-citation-id")).toBe("1");
  });

  it("sanitizes replacement URLs before returning markup", () => {
    const rendered = renderMarkdownWithCitations(`![QR](${attachment.name})`, undefined, [
      { ...attachment, preview_url: "javascript:alert(1)" },
    ]);
    expect(rendered.html).not.toContain("javascript:");
  });
});

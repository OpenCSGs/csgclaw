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
  source_path: "images/feishu-im-auth-qrcode.png",
  workspace_path: ".csgclaw/attachments/qr-image-feishu-im-auth-qrcode.png",
  download_url: "/api/v1/attachments/qr-image?token=image-token",
};

describe("inline image attachments", () => {
  it.each([
    "feishu-im-auth-qrcode.png",
    "./feishu-im-auth-qrcode.png",
    "images/feishu-im-auth-qrcode.png",
    "feishu-im-auth-qrcode.png?version=1#preview",
    "./.csgclaw/attachments/qr-image-feishu-im-auth-qrcode.png",
  ])("resolves the delivered image for %s", (source) => {
    expect(resolveImageAttachmentURL(source, [attachment])).toBe("api/v1/attachments/qr-image?token=image-token");
  });

  it.each(["images/./qr.png", "images/tmp/../qr.png", "images//qr.png", "images/%2E/qr.png"])(
    "renders the published image for the equivalent Markdown path %s",
    (source) => {
      const image = { ...attachment, name: "chart.png", source_path: "images/qr.png" };
      const rendered = renderMarkdownWithCitations(`![QR](${source})`, undefined, [image]);
      expect(rendered.html).toContain('src="api/v1/attachments/qr-image?token=image-token"');
    },
  );

  it.each(["source_path", "workspace_path"] as const)("normalizes the recorded %s before matching", (field) => {
    const image = { ...attachment, [field]: "images/tmp/.././qr.png" };
    expect(resolveImageAttachmentURL("images/qr.png", [image])).toBe("api/v1/attachments/qr-image?token=image-token");
  });

  it.each(["../qr.png", "images/%2E%2E/../qr.png", "/images/../qr.png", "https://example.com/../../qr.png"])(
    "keeps non-workspace references unchanged when an image has the same name: %s",
    (source) => {
      const image = { ...attachment, name: "qr.png", source_path: "qr.png" };
      expect(resolveImageAttachmentURL(source, [image])).toBeUndefined();
    },
  );

  it("requires an exact path for a directory-qualified reference that simplifies to a filename", () => {
    const image = { ...attachment, name: "qr.png", source_path: "generated/qr.png" };
    expect(resolveImageAttachmentURL("assets/../qr.png", [image])).toBeUndefined();
    expect(resolveImageAttachmentURL("assets/../qr.png", [{ ...image, source_path: "qr.png" }])).toBe(
      "api/v1/attachments/qr-image?token=image-token",
    );
  });

  it("matches percent-encoded Unicode names and spaces", () => {
    const image = { ...attachment, name: "chart.png", source_path: "images/飞书 授权二维码.png" };
    expect(resolveImageAttachmentURL(`images/${encodeURIComponent("飞书 授权二维码.png")}`, [image])).toBe(
      "api/v1/attachments/qr-image?token=image-token",
    );
  });

  it("resolves published paths when delivery names collide", () => {
    const first = { ...attachment, name: "qr.png", source_path: "first/qr.png" };
    const second = {
      ...first,
      id: "qr-other",
      source_path: "second/qr.png",
      preview_url: "/api/v1/attachments/qr-other?token=other-token",
    };
    const files = [first, second];
    expect(resolveImageAttachmentURL(first.source_path, files)).toBe("api/v1/attachments/qr-image?token=image-token");
    expect(resolveImageAttachmentURL("./second/qr.png", files)).toBe("api/v1/attachments/qr-other?token=other-token");
    expect(resolveImageAttachmentURL("qr.png", files)).toBeUndefined();
    expect(resolveImageAttachmentURL("third/qr.png", files)).toBeUndefined();
    expect(
      resolveImageAttachmentURL(first.source_path, [first, { ...second, source_path: first.source_path }]),
    ).toBeUndefined();
  });

  it("preserves application image URLs when an attachment shares their filename", () => {
    const image = { ...attachment, name: "manager.png", source_path: "generated/manager.png" };
    const content = "![Avatar](/avatar/manager.png) ![Icon](avatar/manager.png) ![Output](generated/manager.png)";
    const rendered = renderMarkdownWithCitations(content, undefined, [image]);
    const template = document.createElement("template");
    template.innerHTML = rendered.html;
    expect([...template.content.querySelectorAll("img")].map((item) => item.getAttribute("src"))).toEqual([
      "/avatar/manager.png",
      "avatar/manager.png",
      "api/v1/attachments/qr-image?token=image-token",
    ]);
  });

  it.each(["generated/100%.png", "generated/100%25.png"])("renders a published percent sign in %s", (source) => {
    const image = { ...attachment, name: "100%.png", source_path: "generated/100%.png" };
    const rendered = renderMarkdownWithCitations(`![Chart](${source})`, undefined, [image]);
    expect(rendered.html).toContain('src="api/v1/attachments/qr-image?token=image-token"');
  });

  it.each([
    "/workspace/feishu-im-auth-qrcode.png",
    "C:\\workspace\\feishu-im-auth-qrcode.png",
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

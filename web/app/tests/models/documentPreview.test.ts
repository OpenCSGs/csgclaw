import {
  clampDocumentPreviewPanelWidth,
  copyPreviewBuffer,
  documentPreviewKind,
  formatPreviewText,
} from "@/components/business/DocumentPreviewPanel";

describe("document preview helpers", () => {
  it.each([
    ["application/pdf", "file.bin", "pdf"],
    ["application/octet-stream", "report.md", "markdown"],
    ["application/vnd.openxmlformats-officedocument.wordprocessingml.document", "file", "docx"],
    ["application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "file", "spreadsheet"],
    ["application/vnd.openxmlformats-officedocument.presentationml.presentation", "file", "powerpoint"],
    ["application/vnd.ms-powerpoint", "slides.ppt", "unsupported"],
  ])("maps %s %s to %s", (mediaType, name, expected) => {
    expect(documentPreviewKind({ mediaType, name })).toBe(expected);
  });

  it("pretty prints valid JSON and preserves invalid JSON", () => {
    expect(formatPreviewText(new TextEncoder().encode('{"ok":true}').buffer, "application/json")).toBe(
      '{\n  "ok": true\n}',
    );
    expect(formatPreviewText(new TextEncoder().encode("{").buffer, "application/json")).toBe("{");
  });

  it.each([
    ["settings.toml", 'title = "CSGClaw"\n[server]\nport = 18080'],
    ["workflow.yaml", "name: preview\nsteps:\n  - run: test"],
    ["script.unknown", "#!/bin/sh\necho hello"],
  ])("detects %s as text even when its MIME type is generic", (name, content) => {
    const data = new TextEncoder().encode(content).buffer;
    expect(documentPreviewKind({ mediaType: "application/octet-stream", name }, data)).toBe("text");
    expect(formatPreviewText(data, "application/octet-stream")).toBe(content);
  });

  it("supports BOM-marked UTF-16 text and rejects unknown binary data", () => {
    const utf16 = new Uint8Array([0xff, 0xfe, 0x68, 0x00, 0x69, 0x00]).buffer;
    expect(documentPreviewKind({ mediaType: "application/octet-stream", name: "notes.data" }, utf16)).toBe("text");
    expect(formatPreviewText(utf16, "application/octet-stream")).toBe("hi");

    const binary = new Uint8Array([0x50, 0x4b, 0x03, 0x04, 0x00, 0xff, 0x00, 0x81]).buffer;
    expect(documentPreviewKind({ mediaType: "application/octet-stream", name: "archive.data" }, binary)).toBe(
      "unsupported",
    );
  });

  it("clamps the panel against its actual content container", () => {
    expect(clampDocumentPreviewPanelWidth(1600, 1280)).toBe(920);
    expect(clampDocumentPreviewPanelWidth(200, 1280)).toBe(420);
    expect(clampDocumentPreviewPanelWidth(640, 700)).toBe(340);
  });

  it("isolates transferable renderer data and safely handles detached buffers", () => {
    const original = new TextEncoder().encode('title = "CSGClaw"').buffer;
    const rendererCopy = copyPreviewBuffer(original);
    structuredClone(rendererCopy, { transfer: [rendererCopy] });
    expect(rendererCopy.byteLength).toBe(0);
    expect(formatPreviewText(original, "application/toml")).toBe('title = "CSGClaw"');

    structuredClone(original, { transfer: [original] });
    expect(() =>
      documentPreviewKind({ mediaType: "application/octet-stream", name: "revive.toml" }, original),
    ).not.toThrow();
    expect(documentPreviewKind({ mediaType: "application/octet-stream", name: "revive.toml" }, original)).toBe(
      "unsupported",
    );
    expect(formatPreviewText(original, "application/toml")).toBe("");
  });
});

import { formatPreviewText } from "@/components/business/DocumentPreviewPanel";
import samples from "../fixtures/textEncodings.json";

function bytesFromBase64(value: string): Uint8Array<ArrayBuffer> {
  return Uint8Array.from(atob(value), (character) => character.charCodeAt(0));
}

describe("text preview encodings", () => {
  it.each(samples)("decodes $encoding without damaging Chinese or ASCII", ({ base64, text }) => {
    expect(formatPreviewText(bytesFromBase64(base64).buffer, "text/markdown")).toBe(text);
  });

  it("finds encoded content after a long ASCII preamble", () => {
    const encoded = bytesFromBase64(samples[0]!.base64);
    const data = new Uint8Array(128 * 1024 + encoded.length);
    data.fill(0x20);
    data.set(encoded, 128 * 1024);
    expect(formatPreviewText(data.buffer, "text/plain")).toBe(" ".repeat(128 * 1024) + samples[0]!.text);
  });

  it("honors quoted charset declarations for ambiguous short text", () => {
    const data = new Uint8Array([0xa4, 0xa4, 0xa4, 0xe5]).buffer;
    expect(formatPreviewText(data, 'text/plain; charset="big5"')).toBe("中文");
    expect(formatPreviewText(data, "text/plain", { contentType: "text/plain; CHARSET=Big5" })).toBe("中文");
  });

  it("prefers the BOM over a contradictory charset declaration", () => {
    const data = new Uint8Array([0xef, 0xbb, 0xbf, ...new TextEncoder().encode("中文")]).buffer;
    expect(formatPreviewText(data, "text/markdown; charset=gbk")).toBe("中文");
  });

  it("falls back to detection for unsupported or invalid declared encodings", () => {
    const data = bytesFromBase64(samples[0]!.base64).buffer;
    expect(formatPreviewText(data, "text/plain; charset=not-an-encoding")).toBe(samples[0]!.text);
    expect(formatPreviewText(data, "text/plain; charset=utf-8")).toBe(samples[0]!.text);
  });

  it("preserves valid UTF-8, including existing replacement characters", () => {
    const text = "# 中文报告 😀\nOriginal � corruption";
    expect(formatPreviewText(new TextEncoder().encode(text).buffer, "text/markdown")).toBe(text);
  });

  it("retains JSON formatting when the response declares a different charset", () => {
    const data = new Uint8Array([0x7b, 0x22, 0xa4, 0xa4, 0xa4, 0xe5, 0x22, 0x3a, 0x31, 0x7d]).buffer;
    expect(formatPreviewText(data, "application/json", { contentType: "text/plain; charset=big5" })).toBe(
      '{\n  "中文": 1\n}',
    );
  });

  it.each(["utf-8", "gb18030"])("ignores only the incomplete tail of a truncated %s preview", (encoding) => {
    const text = samples[0]!.text;
    const encoded = encoding === "utf-8" ? new TextEncoder().encode(text) : bytesFromBase64(samples[0]!.base64);
    const truncated = encoded.slice(0, -1);
    expect(formatPreviewText(truncated.buffer, "text/markdown", { truncated: true })).toBe(text.slice(0, -1));
  });
});

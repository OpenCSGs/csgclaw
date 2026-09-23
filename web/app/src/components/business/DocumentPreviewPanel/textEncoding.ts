import { detect } from "jschardet";

const MAX_ENCODING_SAMPLE_BYTES = 64 * 1024;

// Statistical detection is only used for known text previews. Unknown binary
// attachments must still pass the stricter Unicode sniff in previewTypes.
export function decodePreviewText(data: ArrayBuffer, mediaType: string, truncated = false): string {
  const bytes = new Uint8Array(data);
  const bomEncoding = detectBOM(bytes);
  if (bomEncoding) {
    return new TextDecoder(bomEncoding).decode(bytes, { stream: truncated });
  }

  const charset = /(?:^|;)\s*charset\s*=\s*(?:"([^"]+)"|([^;\s]+))/i.exec(mediaType);
  const declaredEncoding = charset?.[1] ?? charset?.[2];
  if (declaredEncoding) {
    const text = tryDecode(bytes, declaredEncoding, truncated);
    if (text !== null) {
      return text;
    }
  }

  // Valid UTF-8, including literal replacement characters already in the file,
  // must never be reinterpreted as another encoding.
  const utf8 = tryDecode(bytes, "utf-8", truncated);
  if (utf8 !== null) {
    return utf8;
  }

  // Skip an ASCII preamble so a long header does not hide the encoded content.
  // Keep detection bounded even when the preview contains tens of megabytes.
  const start = bytes.findIndex((byte) => byte >= 0x80);
  const sample = bytes.subarray(Math.max(0, start), Math.max(0, start) + MAX_ENCODING_SAMPLE_BYTES);
  if (!sample.includes(0)) {
    let binary = "";
    for (let offset = 0; offset < sample.length; offset += 8192) {
      binary += String.fromCharCode(...sample.subarray(offset, offset + 8192));
    }
    const detected = detect(binary, { minimumThreshold: 0.5 });
    if (detected.encoding) {
      // The detector calls this family GB2312; GB18030 also decodes GBK and
      // GB2312, while retaining GB18030's four-byte characters.
      const encoding = /^(gb2312|gbk|gb18030)$/i.test(detected.encoding) ? "gb18030" : detected.encoding;
      const text = tryDecode(bytes, encoding, truncated);
      if (text !== null) {
        return text;
      }
    }
  }
  return new TextDecoder("utf-8").decode(bytes, { stream: truncated });
}

function detectBOM(bytes: Uint8Array): string | null {
  if (bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf) {
    return "utf-8";
  }
  if (bytes[0] === 0xff && bytes[1] === 0xfe) {
    return "utf-16le";
  }
  if (bytes[0] === 0xfe && bytes[1] === 0xff) {
    return "utf-16be";
  }
  return null;
}

function tryDecode(bytes: Uint8Array, encoding: string, truncated: boolean): string | null {
  try {
    return new TextDecoder(encoding, { fatal: true }).decode(bytes, { stream: truncated });
  } catch {
    return null;
  }
}

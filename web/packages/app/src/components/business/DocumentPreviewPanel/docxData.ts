import JSZip from "jszip";

// Windows archive tools can store backslash entry names and UTF-16 XML parts.
// Mammoth expects slash-separated package names and reads XML as UTF-8.
export async function prepareDocxData(data: ArrayBuffer): Promise<ArrayBuffer> {
  const zip = await JSZip.loadAsync(data);
  const files = Object.values(zip.files);
  const names = new Set<string>();
  const xmlParts = new Map<string, string>();
  let changed = false;
  for (const file of files) {
    const name = file.name.replace(/\\/g, "/");
    if (names.has(name)) throw new Error("Ambiguous DOCX package entry");
    names.add(name);
    if (name !== file.name) changed = true;
    if (file.dir || !/\.(xml|rels)$/i.test(file.name)) continue;
    const bytes = await file.async("uint8array");
    const encoding = xmlUTF16Encoding(bytes);
    if (!encoding) continue;
    const xml = new TextDecoder(encoding, { fatal: true })
      .decode(bytes)
      .replace(/^(<\?xml\s[^?]*\bencoding\s*=\s*)(["'])[^"']*\2/i, '$1"UTF-8"');
    xmlParts.set(file.name, xml);
    changed = true;
  }
  if (!changed) return data;
  const normalized = new JSZip();
  for (const file of files) {
    normalized.file(file.name.replace(/\\/g, "/"), xmlParts.get(file.name) ?? file.async("uint8array"), {
      dir: file.dir,
      createFolders: false,
      date: file.date,
      comment: file.comment,
    });
  }
  return normalized.generateAsync({ type: "arraybuffer" });
}

function xmlUTF16Encoding(bytes: Uint8Array): string | null {
  if ((bytes[0] === 0xff && bytes[1] === 0xfe) || (bytes[0] === 0x3c && bytes[1] === 0)) {
    return "utf-16le";
  }
  if ((bytes[0] === 0xfe && bytes[1] === 0xff) || (bytes[0] === 0 && bytes[1] === 0x3c)) {
    return "utf-16be";
  }
  return null;
}

import JSZip from "jszip";

// PowerShell's file output can produce valid UTF-16 XML parts. Mammoth reads
// every part as UTF-8, so transcode these parts before handing it the package.
export async function prepareDocxData(data: ArrayBuffer): Promise<ArrayBuffer> {
  const zip = await JSZip.loadAsync(data);
  let changed = false;
  for (const file of Object.values(zip.files)) {
    if (file.dir || !/\.(xml|rels)$/i.test(file.name)) continue;
    const bytes = await file.async("uint8array");
    const encoding = xmlUTF16Encoding(bytes);
    if (!encoding) continue;
    const xml = new TextDecoder(encoding, { fatal: true })
      .decode(bytes)
      .replace(/^(<\?xml\s[^?]*\bencoding\s*=\s*)(["'])[^"']*\2/i, '$1"UTF-8"');
    zip.file(file.name, xml);
    changed = true;
  }
  return changed ? zip.generateAsync({ type: "arraybuffer" }) : data;
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

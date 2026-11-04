import JSZip from "jszip";
import { prepareDocxData } from "@/components/business/DocumentPreviewPanel/docxData";

function utf16(text: string, littleEndian: boolean, bom: boolean): Uint8Array<ArrayBuffer> {
  const data = new Uint8Array((text.length + Number(bom)) * 2);
  const view = new DataView(data.buffer);
  if (bom) view.setUint16(0, 0xfeff, littleEndian);
  for (let index = 0; index < text.length; index += 1) {
    view.setUint16((index + Number(bom)) * 2, text.charCodeAt(index), littleEndian);
  }
  return data;
}

describe("DOCX XML encoding", () => {
  it.each([
    [true, true],
    [false, true],
    [true, false],
    [false, false],
  ])("decodes UTF-16 XML and relationships (little endian %s, BOM %s)", async (littleEndian, bom) => {
    const xml = '<?xml version="1.0" encoding="UTF-16"?><document>春望 😀</document>';
    const zip = new JSZip();
    zip.file("word/document.xml", utf16(xml, littleEndian, bom));
    zip.file("_rels/.rels", utf16(xml, littleEndian, bom));
    const image = new Uint8Array([0, 255, 10, 128]);
    zip.file("word/media/picture.png", image);
    const data = await zip.generateAsync({ type: "arraybuffer" });
    const normalized = await JSZip.loadAsync(await prepareDocxData(data));
    for (const name of ["word/document.xml", "_rels/.rels"]) {
      expect(await normalized.file(name)!.async("string")).toBe(xml.replace("UTF-16", "UTF-8"));
    }
    expect(await normalized.file("word/media/picture.png")!.async("uint8array")).toEqual(image);
  });

  it("passes a UTF-8 package through without rewriting it", async () => {
    const data = await new JSZip()
      .file("word/document.xml", '<?xml version="1.0" encoding="UTF-8"?><document>春望</document>')
      .generateAsync({ type: "arraybuffer" });
    expect(await prepareDocxData(data)).toBe(data);
  });

  it("rejects a corrupt package", async () => {
    await expect(prepareDocxData(new Uint8Array([1, 2, 3]).buffer)).rejects.toThrow();
  });
});

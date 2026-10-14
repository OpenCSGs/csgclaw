import JSZip from "jszip";

const presentationML = "http://schemas.openxmlformats.org/presentationml/2006/main";
const drawingML = "http://schemas.openxmlformats.org/drawingml/2006/main";

// defaultTextStyle is optional in PPTX, but the renderer assumes an object.
// Supply it in the preview copy only; downloads retain the original bytes.
export async function normalizePowerPointPreview(data: ArrayBuffer): Promise<ArrayBuffer> {
  const zip = await JSZip.loadAsync(data);
  const presentation = zip.file("ppt/presentation.xml");
  if (!presentation) throw new Error("PowerPoint presentation is missing");
  const xml = new DOMParser().parseFromString(await presentation.async("string"), "application/xml");
  if (xml.querySelector("parsererror")) throw new Error("Invalid PowerPoint presentation");
  if (xml.getElementsByTagNameNS(presentationML, "defaultTextStyle").length) return data;
  const styles = xml.createElementNS(presentationML, "p:defaultTextStyle");
  const level = xml.createElementNS(drawingML, "a:lvl1pPr");
  level.appendChild(xml.createElementNS(drawingML, "a:defRPr"));
  styles.appendChild(level);
  xml.documentElement.appendChild(styles);
  zip.file("ppt/presentation.xml", new XMLSerializer().serializeToString(xml));
  return zip.generateAsync({ type: "arraybuffer" });
}

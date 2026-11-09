import JSZip from "jszip";

const presentationML = "http://schemas.openxmlformats.org/presentationml/2006/main";
const contentTypes = "http://schemas.openxmlformats.org/package/2006/content-types";
const drawingML = "http://schemas.openxmlformats.org/drawingml/2006/main";

// Repair renderer assumptions in the preview copy only; downloads retain the original bytes.
export async function normalizePowerPointPreview(data: ArrayBuffer): Promise<ArrayBuffer> {
  const zip = await JSZip.loadAsync(data);
  const presentation = zip.file("ppt/presentation.xml");
  if (!presentation) throw new Error("PowerPoint presentation is missing");
  const xml = new DOMParser().parseFromString(await presentation.async("string"), "application/xml");
  if (xml.querySelector("parsererror")) throw new Error("Invalid PowerPoint presentation");
  let changed = false;
  if (!xml.getElementsByTagNameNS(presentationML, "defaultTextStyle").length) {
    const styles = xml.createElementNS(presentationML, "p:defaultTextStyle");
    const level = xml.createElementNS(drawingML, "a:lvl1pPr");
    level.appendChild(xml.createElementNS(drawingML, "a:defRPr"));
    styles.appendChild(level);
    xml.documentElement.appendChild(styles);
    zip.file("ppt/presentation.xml", new XMLSerializer().serializeToString(xml));
    changed = true;
  }
  // Some generators leave declarations for deleted masters. The renderer loads
  // every Override and silently stops before reading slides if a part is absent.
  const manifest = zip.file("[Content_Types].xml");
  if (manifest) {
    const types = new DOMParser().parseFromString(await manifest.async("string"), "application/xml");
    if (types.querySelector("parsererror")) throw new Error("Invalid PowerPoint content types");
    let removed = false;
    for (const entry of Array.from(types.getElementsByTagNameNS(contentTypes, "Override"))) {
      const partName = entry.getAttribute("PartName");
      if (partName && !zip.file(partName.replace(/^\//, ""))) {
        entry.remove();
        removed = true;
      }
    }
    if (removed) {
      zip.file("[Content_Types].xml", new XMLSerializer().serializeToString(types));
      changed = true;
    }
  }
  return changed ? zip.generateAsync({ type: "arraybuffer" }) : data;
}

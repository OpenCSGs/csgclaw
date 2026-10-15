import JSZip from "jszip";
import { normalizePowerPointPreview } from "@/components/business/DocumentPreviewPanel/normalizePowerPoint";

const prefix =
  '<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">';
async function presentation(content: string) {
  const zip = new JSZip();
  zip.file("ppt/presentation.xml", prefix + content + "</p:presentation>");
  return zip.generateAsync({ type: "arraybuffer" });
}

it("supplies missing optional text styles in the preview copy", async () => {
  const original = await presentation('<p:sldSz cx="9144000" cy="6858000"/>');
  const output = await normalizePowerPointPreview(original);
  const zip = await JSZip.loadAsync(output);
  const xml = await zip.file("ppt/presentation.xml")!.async("string");
  expect(xml).toContain("p:defaultTextStyle");
  expect(xml).toContain("a:lvl1pPr");
  const originalZip = await JSZip.loadAsync(original);
  expect(await originalZip.file("ppt/presentation.xml")!.async("string")).not.toContain("p:defaultTextStyle");
});

it("preserves presentations that already define default styles", async () => {
  const original = await presentation("<p:defaultTextStyle><a:lvl1pPr/></p:defaultTextStyle>");
  expect(await normalizePowerPointPreview(original)).toBe(original);
});

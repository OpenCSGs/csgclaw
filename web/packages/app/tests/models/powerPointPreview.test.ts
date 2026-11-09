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

it("removes declarations for absent parts without changing the original package", async () => {
  const zip = await JSZip.loadAsync(await presentation("<p:defaultTextStyle><a:lvl1pPr/></p:defaultTextStyle>"));
  const manifest =
    '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">' +
    '<Default Extension="xml" ContentType="application/xml"/>' +
    '<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>' +
    '<Override PartName="/ppt/slideMasters/slideMaster2.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>' +
    '<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>' +
    "</Types>";
  zip.file("[Content_Types].xml", manifest);
  zip.file("ppt/slideMasters/slideMaster1.xml", "<master/>");
  zip.file("ppt/slides/slide1.xml", "<slide/>");
  const original = await zip.generateAsync({ type: "arraybuffer" });
  const normalized = await JSZip.loadAsync(await normalizePowerPointPreview(original));
  const xml = await normalized.file("[Content_Types].xml")!.async("string");
  expect(xml).toContain("slideMaster1.xml");
  expect(xml).not.toContain("slideMaster2.xml");
  expect(xml).toContain("slide1.xml");
  expect(xml).toContain("<Default");
  expect(await normalized.file("ppt/slides/slide1.xml")!.async("string")).toBe("<slide/>");
  expect(await (await JSZip.loadAsync(original)).file("[Content_Types].xml")!.async("string")).toBe(manifest);
  const repaired = await normalizePowerPointPreview(original);
  expect(await normalizePowerPointPreview(repaired)).toBe(repaired);
});

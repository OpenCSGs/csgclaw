import { syntaxLanguageForFile } from "@/components/business/DocumentPreviewPanel";

describe("syntaxLanguageForFile", () => {
  it.each([
    ["main.go", "text/plain", "go"],
    ["settings.toml", "application/octet-stream", "toml"],
    ["workflow.yaml", "application/octet-stream", "yaml"],
    ["Dockerfile", "text/plain", "dockerfile"],
    ["Makefile", "text/plain", "makefile"],
    ["component.tsx", "text/plain", "tsx"],
    ["unknown", "application/json", "json"],
    ["README", "text/plain", null],
  ])("maps %s with %s to %s", (name, mediaType, expected) => {
    expect(syntaxLanguageForFile(name, mediaType)).toBe(expected);
  });
});

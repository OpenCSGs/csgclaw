import { readFileSync } from "node:fs";
import { resolve } from "node:path";

function readSource(path: string): string {
  return readFileSync(resolve(process.cwd(), path), "utf8");
}

describe("default app scale", () => {
  it("renders the full UI at normal viewport scale", () => {
    const globals = readSource("src/shared/styles/globals.css");
    const workspace = readSource("src/pages/WorkspacePage/components/WorkspaceComponents.css");
    const workspaceLayout = readSource("src/pages/WorkspacePage/components/WorkspaceLayout/WorkspaceLayout.module.css");
    const settings = readSource("src/pages/SettingsPage/SettingsPage.module.css");

    expect(globals).toContain("--app-ui-viewport-height: 100dvh;");
    expect(globals).not.toContain("--app-ui-floating-layer-scale");
    expect(globals).not.toContain("@supports (zoom: 80%)");
    expect(globals).not.toContain("--app-ui-viewport-height: 125dvh;");
    expect(globals).not.toContain("font-size-adjust: 0.65;");
    expect(globals).not.toContain("zoom: 80%;");
    expect(globals).not.toContain("[data-radix-popper-content-wrapper]");
    expect(globals).toContain("font-size: var(--text-md-size);");
    expect(globals).toContain("font-family: var(--font-sans);");
    expect(workspace).toContain("height: var(--app-ui-viewport-height);");
    expect(workspaceLayout.match(/height: var\(--app-ui-viewport-height\);/g)).toHaveLength(3);
    expect(settings).toContain("min-height: var(--app-ui-viewport-height);");
  });
});

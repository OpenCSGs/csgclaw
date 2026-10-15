import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { render, screen } from "@testing-library/react";
import { Button, Checkbox, Field, LinkButton, Select, Switch, TextArea, TextInput } from "@/components/ui";
import { skinRulesMatching, stylesheetSource, styleRules } from "../helpers/styleRules";

function filesIn(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name);
    return entry.isDirectory() ? filesIn(path) : [path];
  });
}

const uiFiles = filesIn(resolve("src/components/ui"));

afterEach(() => {
  delete document.documentElement.dataset.theme;
});

test("component styles are local Modules with semantic theme values", () => {
  const cssFiles = uiFiles.filter((file) => file.endsWith(".css"));
  expect(cssFiles.length).toBeGreaterThan(0);
  for (const file of cssFiles) {
    expect(file).toMatch(/\.module\.css$/);
    const source = readFileSync(file, "utf8");
    expect(source, file).not.toMatch(/:global\(|data-theme|!important|#[\da-f]{3,8}\b|(?:rgba?|hsla?|oklch)\(/i);
  }
  for (const file of uiFiles.filter((file) => file.endsWith("/index.ts"))) {
    expect(readFileSync(file, "utf8"), file).not.toMatch(/import\s+["'][^"']+\.css["']/);
  }
});

test.each(["light", "dark"])("legacy workspace skins cannot restyle managed controls in %s mode", (theme) => {
  document.documentElement.dataset.theme = theme;
  render(
    <div className="modal-card profile-modal agent-modal">
      <section className="profile-section profile-runtime-grid agent-model-config-grid">
        <Field label="Managed input" error="Required">
          <TextInput disabled />
        </Field>
        <Field label="Managed area">
          <TextArea />
        </Field>
        <Field label="Choice">
          <Select value="one" options={[{ value: "one", label: "One" }]} />
        </Field>
        <Button className="icon-button" disabled>
          Managed button
        </Button>
        <LinkButton className="icon-button" href="#fixture">
          Managed link
        </LinkButton>
        <Checkbox aria-label="Managed checkbox" disabled />
        <Switch aria-label="Managed switch" disabled />
        <label className="field">
          <input aria-label="Legacy input" />
        </label>
      </section>
    </div>,
  );
  const legacyCSS = [
    "src/shared/styles/inputs.css",
    "src/pages/WorkspacePage/components/WorkspaceComponents.css",
    "src/pages/WorkspacePage/components/WorkspaceModals/WorkspaceModals.css",
  ].flatMap((file) => styleRules(stylesheetSource(resolve(file))));
  const managed = [
    screen.getByRole("textbox", { name: "Managed input" }),
    screen.getByRole("textbox", { name: "Managed area" }),
    screen.getByRole("combobox", { name: "Choice" }),
    screen.getByRole("button", { name: "Managed button" }),
    screen.getByRole("link", { name: "Managed link" }),
    screen.getByRole("checkbox", { name: "Managed checkbox" }),
    screen.getByRole("switch", { name: "Managed switch" }),
  ];
  for (const control of managed) {
    expect(control).toHaveAttribute("data-csg-control");
    expect(skinRulesMatching(control, legacyCSS), control.outerHTML).toEqual([]);
  }
  expect(skinRulesMatching(screen.getByRole("textbox", { name: "Legacy input" }), legacyCSS).length).toBeGreaterThan(0);
});

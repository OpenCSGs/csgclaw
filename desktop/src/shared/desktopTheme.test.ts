import assert from "node:assert/strict";
import test from "node:test";
import { parseDesktopThemeSource, shouldUseDarkDockIcon } from "./desktopTheme";

test("parseDesktopThemeSource accepts supported theme sources", () => {
  for (const theme of ["system", "light", "dark"] as const) {
    assert.equal(parseDesktopThemeSource(theme), theme);
  }
});

test("parseDesktopThemeSource rejects unsupported values", () => {
  for (const value of [undefined, null, "auto", true, {}]) {
    assert.throws(
      () => parseDesktopThemeSource(value),
      /Desktop theme source is invalid/,
    );
  }
});

test("shouldUseDarkDockIcon gives the app setting precedence", () => {
  assert.equal(shouldUseDarkDockIcon("light", true), false);
  assert.equal(shouldUseDarkDockIcon("light", false), false);
  assert.equal(shouldUseDarkDockIcon("dark", true), true);
  assert.equal(shouldUseDarkDockIcon("dark", false), true);
});

test("shouldUseDarkDockIcon follows macOS in system mode", () => {
  assert.equal(shouldUseDarkDockIcon("system", true), true);
  assert.equal(shouldUseDarkDockIcon("system", false), false);
});

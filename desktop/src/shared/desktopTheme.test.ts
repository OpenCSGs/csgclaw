import assert from "node:assert/strict";
import test from "node:test";
import { parseDesktopThemeSource, shouldUseDarkThemeIcon } from "./desktopTheme";

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

test("shouldUseDarkThemeIcon gives the app setting precedence", () => {
  assert.equal(shouldUseDarkThemeIcon("light", true), false);
  assert.equal(shouldUseDarkThemeIcon("light", false), false);
  assert.equal(shouldUseDarkThemeIcon("dark", true), true);
  assert.equal(shouldUseDarkThemeIcon("dark", false), true);
});

test("shouldUseDarkThemeIcon follows the operating system in system mode", () => {
  assert.equal(shouldUseDarkThemeIcon("system", true), true);
  assert.equal(shouldUseDarkThemeIcon("system", false), false);
});

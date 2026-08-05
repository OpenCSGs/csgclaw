import assert from "node:assert/strict";
import test from "node:test";
import { parseDesktopThemeSource } from "./desktopTheme";

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

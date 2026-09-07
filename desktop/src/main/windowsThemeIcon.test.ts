import assert from "node:assert/strict";
import test from "node:test";
import { windowsThemeIconName } from "./windowsThemeIcon";

test("uses the app theme before the system theme for Windows icons", () => {
  assert.equal(
    windowsThemeIconName("light", true),
    "csgclaw-taskbar-light.ico",
  );
  assert.equal(
    windowsThemeIconName("light", false),
    "csgclaw-taskbar-light.ico",
  );
  assert.equal(
    windowsThemeIconName("dark", true),
    "csgclaw-taskbar-dark.ico",
  );
  assert.equal(
    windowsThemeIconName("dark", false),
    "csgclaw-taskbar-dark.ico",
  );
});

test("follows the system theme for Windows icons only in system mode", () => {
  assert.equal(
    windowsThemeIconName("system", false),
    "csgclaw-taskbar-light.ico",
  );
  assert.equal(
    windowsThemeIconName("system", true),
    "csgclaw-taskbar-dark.ico",
  );
});

import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import {
  readDesktopThemeSourcePreference,
  writeDesktopThemeSourcePreference,
} from "./desktopThemePreference";

function fixture(t: import("node:test").TestContext): string {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "csgclaw-theme-"));
  t.after(() => {
    if (!fs.realpathSync(root).startsWith(fs.realpathSync(os.tmpdir()))) {
      throw new Error(`Refusing to remove unexpected path: ${root}`);
    }
    fs.rmSync(root, { recursive: true, force: true });
  });
  return root;
}

test("uses the app dark theme default before the renderer syncs settings", (t) => {
  const root = fixture(t);
  assert.equal(readDesktopThemeSourcePreference(root), "dark");
});

test("persists the desktop theme source for early tray icon selection", (t) => {
  const root = fixture(t);
  writeDesktopThemeSourcePreference(root, "system");
  assert.equal(readDesktopThemeSourcePreference(root), "system");
  writeDesktopThemeSourcePreference(root, "light");
  assert.equal(readDesktopThemeSourcePreference(root), "light");
  writeDesktopThemeSourcePreference(root, "dark");
  assert.equal(readDesktopThemeSourcePreference(root), "dark");
});

test("falls back to dark when the stored desktop theme source is invalid", (t) => {
  const root = fixture(t);
  const errors: unknown[] = [];
  fs.writeFileSync(path.join(root, "desktop-theme-source.json"), '{"theme":"blue"}');
  assert.equal(readDesktopThemeSourcePreference(root, (error) => errors.push(error)), "dark");
  assert.equal(errors.length, 1);
});

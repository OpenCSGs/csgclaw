import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { DesktopIPC } from "../shared/desktopBridge.types";

test("sandboxed preload has no local runtime dependencies", () => {
  const compiledPreload = fs.readFileSync(path.join(__dirname, "index.js"), "utf8");

  assert.doesNotMatch(compiledPreload, /require\(["']\.\.\//);
  for (const channel of Object.values(DesktopIPC)) {
    assert.match(compiledPreload, new RegExp(channel));
  }
});

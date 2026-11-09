import assert from "node:assert/strict";
import test from "node:test";
import type { BrowserWindow } from "electron";
import { restoreWindowFocusAfterDialog } from "./focusRecovery";

function createWindow(destroyed = false) {
  const calls: string[] = [];
  const window = {
    isDestroyed: () => destroyed,
    blur: () => calls.push("blur"),
    focus: () => calls.push("focus"),
  } as unknown as BrowserWindow;
  return { calls, window };
}

test("restores native window activation after a JavaScript dialog on Windows", () => {
  const { calls, window } = createWindow();

  assert.equal(restoreWindowFocusAfterDialog(window, "win32"), true);
  assert.deepEqual(calls, ["blur", "focus"]);
});

test("does not disturb other platforms or destroyed windows", () => {
  const live = createWindow();
  const destroyed = createWindow(true);

  assert.equal(restoreWindowFocusAfterDialog(live.window, "darwin"), false);
  assert.equal(restoreWindowFocusAfterDialog(destroyed.window, "win32"), false);
  assert.deepEqual(live.calls, []);
  assert.deepEqual(destroyed.calls, []);
});

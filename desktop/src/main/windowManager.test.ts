import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import fs from "node:fs";
import path from "node:path";
import { setImmediate } from "node:timers/promises";
import test from "node:test";
import { runInNewContext } from "node:vm";
import type { WindowManager } from "./windowManager";

function createHarness(loadError?: Error) {
  const windows: TestWindow[] = [];
  const failures: Error[] = [];

  class TestWindow extends EventEmitter {
    readonly webContents = new EventEmitter();
    destroyed = false;
    minimized = false;
    visible = false;
    focused = false;
    url = "";

    constructor() {
      super();
      windows.push(this);
    }

    isDestroyed() {
      return this.destroyed;
    }
    isMinimized() {
      return this.minimized;
    }
    restore() {
      this.minimized = false;
    }
    show() {
      this.visible = true;
    }
    hide() {
      this.visible = false;
    }
    focus() {
      this.focused = true;
    }
    destroy() {
      this.destroyed = true;
      this.emit("closed");
    }
    async loadURL(url: string) {
      if (loadError) throw loadError;
      this.url = url;
      this.emit("ready-to-show");
    }
  }

  const stubs: Record<string, unknown> = {
    electron: {
      BrowserWindow: TestWindow,
      nativeTheme: { shouldUseDarkColors: false },
      session: {
        fromPartition: () => ({
          webRequest: { onBeforeSendHeaders() {} },
          setPermissionRequestHandler() {},
          setPermissionCheckHandler() {},
        }),
      },
    },
    "./platform": {
      isWindowsDesktop: true,
      windowsAppIconPath: () => "csgclaw.ico",
    },
    "./windowChrome": {
      installWindowChrome() {},
      windowChromeOptions: () => ({}),
    },
    "./nativeBrowser": { NativeBrowserManager: class {} },
    "./navigationPolicy": { installNavigationPolicy() {} },
  };
  const loaded = { exports: {} as { WindowManager: typeof WindowManager } };
  const filename = path.join(__dirname, "windowManager.js");
  runInNewContext(
    fs.readFileSync(filename, "utf8"),
    {
      exports: loaded.exports,
      module: loaded,
      require: (id: string) => stubs[id] ?? require(id),
      __dirname,
      process: { platform: "win32", env: {} },
      URL,
    },
    { filename },
  );
  const manager = new loaded.exports.WindowManager({
    shouldQuit: () => false,
    onLoadFailure: (error) => failures.push(error),
  });
  return { manager, windows, failures };
}

test("activation during backend startup waits for the configured connection", async () => {
  const { manager, windows, failures } = createHarness();

  manager.showAndFocus();
  manager.showAndFocus();
  await setImmediate();

  assert.deepEqual(failures, []);
  assert.equal(windows.length, 0);

  manager.setConnection("http://127.0.0.1:18080", "test-session-token");
  await manager.open();

  assert.equal(windows.length, 1);
  assert.equal(windows[0]?.url, "http://127.0.0.1:18080/");
  assert.equal(windows[0]?.visible, true);
  assert.deepEqual(failures, []);
});

test("activation restores the window and reopens it after destruction", async () => {
  const { manager, windows, failures } = createHarness();
  manager.setConnection("http://127.0.0.1:18080", "test-session-token");
  await manager.open();
  const window = windows[0]!;
  window.minimized = true;
  window.hide();

  manager.showAndFocus();
  assert.equal(windows.length, 1);
  assert.equal(window.minimized, false);
  assert.equal(window.visible, true);
  assert.equal(window.focused, true);

  manager.destroy();
  manager.showAndFocus();
  await setImmediate();
  assert.equal(windows.length, 2);
  assert.equal(windows[1]?.visible, true);
  assert.deepEqual(failures, []);
});

test("opening a window still requires a backend connection", async () => {
  const { manager } = createHarness();
  await assert.rejects(manager.open(), {
    message: "Desktop backend connection is not configured.",
  });
});

test("window load failures reach the recovery handler after connection", async () => {
  const loadError = new Error("backend page failed to load");
  const { manager, failures } = createHarness(loadError);
  manager.setConnection("http://127.0.0.1:18080", "test-session-token");

  manager.showAndFocus();
  await setImmediate();

  assert.deepEqual(failures, [loadError]);
});

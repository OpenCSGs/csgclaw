import assert from "node:assert/strict";
import test from "node:test";
import { EventEmitter } from "node:events";
import fs from "node:fs";
import path from "node:path";
import { runInNewContext } from "node:vm";
import { DesktopIPC } from "../shared/desktopBridge.types";
// The options are pure; Electron's Node module export is not used by this helper.
import { isDesktopReloadShortcut, windowChromeOptions } from "./windowChrome";
test("macOS chrome keeps native traffic lights inside the application toolbar", () => {
  const options = windowChromeOptions("darwin", true);
  assert.equal(options.titleBarStyle, "hidden");
  assert.deepEqual(options.trafficLightPosition, { x: 16, y: 18 });
  assert.equal(options.titleBarOverlay, undefined);
  assert.equal(options.backgroundColor, "#0c111d");
});
test("Windows and Linux use native overlay controls that follow the theme", () => {
  for (const platform of ["win32", "linux"] as const) {
    const dark = windowChromeOptions(platform, true),
      light = windowChromeOptions(platform, false);
    assert.deepEqual(dark.titleBarOverlay, {
      height: 48,
      color: "#0c111d",
      symbolColor: "#d0d5dd",
    });
    assert.deepEqual(light.titleBarOverlay, {
      height: 48,
      color: "#ffffff",
      symbolColor: "#344054",
    });
    assert.equal(dark.autoHideMenuBar, true);
  }
});

test("onboarding controls match the paper background even when the application is dark", () => {
  const options = windowChromeOptions("win32", true, "onboarding");
  assert.equal(options.backgroundColor, "#f4f5ee");
  assert.deepEqual(options.titleBarOverlay, {
    height: 48,
    color: "#f4f5ee",
    symbolColor: "#344054",
  });
});

function createChromeHarness() {
  let destroyed = false;
  let focused = true;
  type Sender = { sender: unknown; senderFrame: { url: string } };
  const handlers = new Map<
    string,
    (event: Sender, input?: unknown) => unknown
  >();
  const overlays: unknown[] = [];
  let reloads = 0;
  let rendererFocuses = 0;
  const webContents = Object.assign(new EventEmitter(), {
    mainFrame: { url: "http://localhost:18080/" },
    isDestroyed: () => destroyed,
    send: () => undefined,
    reload: () => reloads++,
    focus: () => rendererFocuses++,
  });
  const window = Object.assign(new EventEmitter(), {
    webContents,
    isDestroyed: () => destroyed,
    isFocused: () => focused,
    isFullScreen: () => false,
    setBackgroundColor: () => undefined,
    setTitleBarOverlay: (value: unknown) => overlays.push(value),
    destroy: () => {
      destroyed = true;
      window.emit("closed");
    },
  });
  Object.defineProperty(window, "webContents", {
    get: () => {
      if (destroyed) throw new TypeError("Object has been destroyed");
      return webContents;
    },
  });
  const nativeTheme = Object.assign(new EventEmitter(), {
    shouldUseDarkColors: true,
    themeSource: "system",
  });
  const stubs: Record<string, unknown> = {
    electron: {
      nativeTheme,
      ipcMain: {
        handle: (
          channel: string,
          handler: (event: Sender, input?: unknown) => unknown,
        ) => handlers.set(channel, handler),
        removeHandler: (channel: string) => handlers.delete(channel),
      },
    },
    "./navigationPolicy": {
      isTrustedMainFrame: (_sender: unknown, url: string, origin: string) =>
        new URL(url).origin === origin,
    },
  };
  const loaded = {
    exports: {} as {
      installWindowChrome: (window: unknown, origin: () => string) => void;
    },
  };
  runInNewContext(
    fs.readFileSync(path.join(__dirname, "windowChrome.js"), "utf8"),
    {
      exports: loaded.exports,
      module: loaded,
      require: (id: string) => stubs[id] ?? require(id),
      process: { platform: "win32" },
    },
  );
  loaded.exports.installWindowChrome(window, () => "http://localhost:18080");
  const sender = { sender: webContents, senderFrame: webContents.mainFrame };
  const change = (input: unknown, event: Sender = sender) =>
    handlers.get(DesktopIPC.setWindowChromeAppearance)!(event, input);
  return {
    window,
    webContents,
    nativeTheme,
    overlays,
    handlers,
    change,
    reloads: () => reloads,
    setFocused: (value: boolean) => { focused = value; },
    rendererFocuses: () => rendererFocuses,
  };
}

test("temporary chrome survives system-theme changes and restores the latest theme", () => {
  const h = createChromeHarness();
  h.change("onboarding");
  assert.equal((h.overlays.at(-1) as { color: string }).color, "#f4f5ee");
  h.nativeTheme.shouldUseDarkColors = false;
  h.nativeTheme.emit("updated");
  assert.equal((h.overlays.at(-1) as { color: string }).color, "#f4f5ee");
  assert.equal(h.nativeTheme.themeSource, "system");
  h.change("default");
  assert.equal((h.overlays.at(-1) as { color: string }).color, "#ffffff");
  h.nativeTheme.shouldUseDarkColors = true;
  h.nativeTheme.emit("updated");
  assert.equal((h.overlays.at(-1) as { color: string }).color, "#0c111d");
});

test("chrome appearance rejects invalid values, frames and origins", () => {
  const h = createChromeHarness();
  assert.throws(() => h.change("dark"), /appearance was rejected/);
  assert.throws(
    () =>
      h.change("onboarding", {
        sender: {},
        senderFrame: h.webContents.mainFrame,
      }),
    /sender was rejected/,
  );
  assert.throws(
    () =>
      h.change("onboarding", {
        sender: h.webContents,
        senderFrame: { url: "http://localhost:18080/" },
      }),
    /sender was rejected/,
  );
  h.webContents.mainFrame.url = "https://example.com";
  assert.throws(() => h.change("onboarding"), /sender was rejected/);
  assert.equal(h.overlays.length, 0);
});

test("top-level navigation resets chrome", () => {
  const h = createChromeHarness();
  h.change("onboarding");
  h.webContents.emit(
    "did-start-navigation",
    {},
    "http://localhost:18080/frame",
    false,
    false,
  );
  assert.equal((h.overlays.at(-1) as { color: string }).color, "#f4f5ee");
  h.webContents.emit(
    "did-start-navigation",
    {},
    "http://localhost:18080/",
    false,
    true,
  );
  assert.equal((h.overlays.at(-1) as { color: string }).color, "#0c111d");
});

test("window destruction removes IPC and listeners without accessing the destroyed window", () => {
  const h = createChromeHarness();
  assert.doesNotThrow(() => h.window.destroy());
  assert.equal(h.window.isDestroyed(), true);
  assert.equal(h.handlers.size, 0);
  assert.equal(h.nativeTheme.listenerCount("updated"), 0);
  assert.equal(h.webContents.listenerCount("did-start-navigation"), 0);
  assert.equal(h.webContents.listenerCount("before-input-event"), 0);
  assert.equal(h.webContents.listenerCount("did-finish-load"), 0);
});

test("a reloaded document regains focus without activating a background window", () => {
  const h = createChromeHarness();
  h.webContents.emit("did-finish-load");
  assert.equal(h.rendererFocuses(), 1);
  h.setFocused(false);
  h.webContents.emit("did-finish-load");
  assert.equal(h.rendererFocuses(), 1);
});

test("Reload uses the platform shortcut and ignores key-up and modified commands", () => {
  const input = { type: "keyDown", key: "r", control: false, meta: false, alt: false, shift: false } as Electron.Input;
  assert.equal(isDesktopReloadShortcut({ ...input, meta: true }, "darwin"), true);
  assert.equal(isDesktopReloadShortcut({ ...input, control: true }, "darwin"), false);
  for (const platform of ["win32", "linux"] as const) {
    assert.equal(isDesktopReloadShortcut({ ...input, control: true }, platform), true);
    assert.equal(isDesktopReloadShortcut({ ...input, key: "F5" }, platform), true);
    assert.equal(isDesktopReloadShortcut({ ...input, meta: true }, platform), false);
    for (const ignored of [{ type: "keyUp" }, { alt: true }, { shift: true }])
      assert.equal(isDesktopReloadShortcut({ ...input, control: true, ...ignored } as Electron.Input, platform), false);
  }
});

test("Reload only reloads the renderer once per key press", () => {
  const h = createChromeHarness();
  let prevented = 0;
  const event = { preventDefault: () => prevented++ };
  const input = { type: "keyDown", key: "r", control: true, meta: false, alt: false, shift: false };
  h.webContents.emit("before-input-event", event, input);
  h.webContents.emit("before-input-event", event, { ...input, isAutoRepeat: true });
  assert.equal(prevented, 2);
  assert.equal(h.reloads(), 1);
});

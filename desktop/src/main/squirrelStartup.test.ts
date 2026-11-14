import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { runInNewContext } from "node:vm";

for (const command of [
  "--squirrel-install",
  "--squirrel-updated",
  "--squirrel-uninstall",
]) {
  test(`${command} lets the shortcut handler finish before quitting`, () => {
    let quitCalls = 0;
    const app = Object.assign(new EventEmitter(), {
      isPackaged: true,
      enableSandbox() {},
      setName() {},
      setAppUserModelId() {},
      setAppLogsPath() {},
      getVersion: () => "0.11.0-beta.5",
      getPath: (name: string) => `/fixture/${name}`,
      quit: () => {
        quitCalls++;
      },
      requestSingleInstanceLock() {
        throw new Error("Squirrel must not start the application");
      },
    });
    const processStub = Object.assign(new EventEmitter(), {
      arch: "x64",
      platform: "win32",
      windowsStore: false,
      argv: ["CSGClaw.exe", command, "0.11.0-beta5"],
    });
    const stubs: Record<string, unknown> = {
      electron: { app, crashReporter: { start() {} }, dialog: {} },
      "electron-squirrel-startup": true,
      "./appLifecycle": {
        AppLifecycle: class {
          constructor() {
            throw new Error("unexpected lifecycle startup");
          }
        },
      },
      "./platform": { isWindowsDesktop: true },
      "./windowsTaskbar": { windowsAppUserModelID: "fixture" },
      "./desktopLogger": {
        initializeDesktopLogger: () => "/fixture/main.log",
        diagnosticLaunchFlags: () => [command],
        logDesktopInfo() {},
        logDesktopError() {},
      },
    };
    const loaded = { exports: {} };
    const filename = path.join(__dirname, "main.js");
    runInNewContext(
      fs.readFileSync(filename, "utf8"),
      {
        exports: loaded.exports,
        module: loaded,
        require: (id: string) => stubs[id] ?? require(id),
        process: processStub,
      },
      { filename },
    );
    assert.equal(
      quitCalls,
      0,
      "the entrypoint quit before the updater could finish creating/removing shortcuts",
    );
    // electron-squirrel-startup calls this only after its Update.exe child closes.
    app.quit();
    assert.equal(quitCalls, 1);
  });
}

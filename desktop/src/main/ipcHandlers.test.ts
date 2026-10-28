import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { runInNewContext } from "node:vm";
import { DesktopIPC } from "../shared/desktopBridge.types";
import type { registerIPCHandlers } from "./ipcHandlers";

function createHarness(response = 0) {
  const handlers = new Map<string, (...args: unknown[]) => unknown>();
  const opened: string[] = [];
  const confirmations: unknown[] = [];
  const origin = "http://127.0.0.1:18080";
  const frame = { url: origin + "/" };
  const contents = { mainFrame: frame, isDestroyed: () => false };
  const window = { webContents: contents };
  const electron = {
    ipcMain: {
      handle: (channel: string, handler: (...args: unknown[]) => unknown) =>
        handlers.set(channel, handler),
      removeHandler: (channel: string) => handlers.delete(channel),
    },
    shell: {
      openExternal: async (url: string) => {
        opened.push(url);
      },
    },
    dialog: {
      showMessageBox: async (_window: unknown, options: unknown) => {
        confirmations.push(options);
        return { response };
      },
    },
  };
  // Load the production handler with only the native Electron APIs replaced.
  const loaded = {
    exports: {} as { registerIPCHandlers: typeof registerIPCHandlers },
  };
  const filename = path.join(__dirname, "ipcHandlers.js");
  runInNewContext(
    fs.readFileSync(filename, "utf8"),
    {
      exports: loaded.exports,
      module: loaded,
      require: (id: string) => (id === "electron" ? electron : require(id)),
      URL,
    },
    { filename },
  );
  loaded.exports.registerIPCHandlers(
    () => window as never,
    () => origin,
    {} as never,
    {} as never,
    async () => {},
    () => {},
    () => {},
  );
  const event = { sender: contents, senderFrame: frame };
  const invoke = async (input: unknown, sender: unknown = event) =>
    handlers.get(DesktopIPC.openOAuth)!(sender, input);
  return { invoke, opened, confirmations };
}

test("OpenCSG HTTP login reaches the browser after custom-site confirmation", async () => {
  const { invoke, opened, confirmations } = createHarness();
  const url =
    "http://aihub.isse.cast/login?return_to=http%3A%2F%2F127.0.0.1%3A18080";
  assert.equal(
    ((await invoke({ purpose: "opencsg-auth", url })) as { opened: boolean })
      .opened,
    true,
  );
  assert.deepEqual(opened, [url]);
  assert.equal(confirmations.length, 1);
});

test("OpenCSG HTTPS login remains supported", async () => {
  const { invoke, opened, confirmations } = createHarness();
  const url = "https://opencsg.com/login";
  await invoke({ purpose: "opencsg-auth", url });
  assert.deepEqual(opened, [url]);
  assert.equal(confirmations.length, 0);
});

test("canceling the HTTP custom-site confirmation does not open a browser", async () => {
  const { invoke, opened, confirmations } = createHarness(1);
  assert.equal(
    (
      (await invoke({
        purpose: "opencsg-auth",
        url: "http://private.example/login",
      })) as { opened: boolean }
    ).opened,
    false,
  );
  assert.equal(confirmations.length, 1);
  assert.deepEqual(opened, []);
});

test("OpenCSG rejects credentials, non-web URLs and untrusted IPC senders", async () => {
  const { invoke, opened, confirmations } = createHarness();
  for (const url of [
    "http://user:password@aihub.isse.cast/login",
    "https://user:password@opencsg.com/login",
    "javascript:alert(1)",
    "file:///etc/passwd",
    "csgclaw-cli://run",
    "/login",
  ]) {
    await assert.rejects(
      invoke({ purpose: "opencsg-auth", url }),
      /OAuth URL must/,
    );
  }
  await assert.rejects(
    invoke(
      { purpose: "opencsg-auth", url: "http://aihub.isse.cast/login" },
      {},
    ),
    /sender was rejected/,
  );
  assert.deepEqual(opened, []);
  assert.equal(confirmations.length, 0);
});

test("connector OAuth retains its HTTPS requirement and GitHub host validation", async () => {
  const { invoke, opened } = createHarness();
  for (const purpose of ["github-connector", "gitlab-connector"]) {
    await assert.rejects(
      invoke({ purpose, url: "http://github.com/login" }),
      /HTTPS URL/,
    );
  }
  await assert.rejects(
    invoke({ purpose: "github-connector", url: "https://other.example/login" }),
    /must use github.com/,
  );
  await invoke({
    purpose: "github-connector",
    url: "https://github.com/login/oauth/authorize",
  });
  assert.deepEqual(opened, ["https://github.com/login/oauth/authorize"]);
});

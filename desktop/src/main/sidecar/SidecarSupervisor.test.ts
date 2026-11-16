import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { PassThrough } from "node:stream";
import { setImmediate } from "node:timers/promises";
import test, { type TestContext } from "node:test";
import { runInNewContext } from "node:vm";
import type { SidecarSupervisor } from "./SidecarSupervisor";

function harness(t: TestContext) {
  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "csgclaw-sidecar-startup-"),
  );
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const child = Object.assign(new EventEmitter(), {
    pid: 12345,
    stdin: new PassThrough(),
    stdout: new PassThrough(),
    stderr: new PassThrough(),
  });
  let bootstrap = "";
  child.stdin.on("data", (chunk: Buffer) => {
    bootstrap += chunk.toString();
  });
  const stubs: Record<string, unknown> = {
    "node:child_process": { spawn: () => child },
    electron: {
      app: { getPath: () => root, getVersion: () => "dev", isPackaged: false },
    },
    "./childEnvironment": { resolveSidecarEnvironment: async () => ({}) },
    "./locateBundle": {
      locateBackendBundle: () => ({ root, executable: "fixture-backend" }),
    },
  };
  const loaded = {
    exports: {} as { SidecarSupervisor: typeof SidecarSupervisor },
  };
  const filename = path.join(__dirname, "SidecarSupervisor.js");
  runInNewContext(
    fs.readFileSync(filename, "utf8"),
    {
      exports: loaded.exports,
      module: loaded,
      require: (id: string) => stubs[id] ?? require(id),
      process: { env: {} },
      setTimeout,
      clearTimeout,
      Buffer,
    },
    { filename },
  );
  const supervisor = new loaded.exports.SidecarSupervisor();
  return {
    supervisor,
    child,
    ready() {
      const sent = JSON.parse(bootstrap.trim());
      child.stdout.write(
        JSON.stringify({
          type: "csgclaw.desktop.ready",
          protocol_version: 1,
          instance_id: sent.instance_id,
          pid: child.pid,
          distribution: "electron",
          version: "dev",
          base_url: "http://127.0.0.1:18080",
        }) + "\n",
      );
    },
  };
}

test("lets a cold backend finish beyond the former 30 second startup limit", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const h = harness(t);
  const outcome = h.supervisor.start().then(
    (value) => ({ value }),
    (error) => ({ error }),
  );
  await setImmediate();
  t.mock.timers.tick(120_000);
  h.ready();
  const result = await outcome;
  assert.ok(
    "value" in result,
    "a backend still initializing must not be killed after 30 seconds",
  );
  assert.equal(h.supervisor.state, "ready");
  h.child.emit("exit", 0, null);
});

test("still bounds a backend that never reports ready", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const h = harness(t);
  let settled = false;
  const outcome = h.supervisor.start().then(
    () => {
      settled = true;
    },
    (error) => {
      settled = true;
      return error;
    },
  );
  await setImmediate();
  t.mock.timers.tick(299_999);
  await setImmediate();
  assert.equal(settled, false);
  t.mock.timers.tick(1);
  assert.match(
    String(await outcome),
    /did not become ready within 300 seconds/,
  );
  h.child.emit("exit", 1, null);
});

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { runStapler } from "./macos-stapler.mjs";

const disconnected = {
  status: 68,
  stderr:
    'Error Domain=NSURLErrorDomain Code=-1005 "The network connection was lost."\nThe validate action failed! Error 68.',
};

function harness(results) {
  const calls = [];
  const waits = [];
  const logs = [];
  return {
    calls,
    waits,
    logs,
    options: {
      environment: { TASK_TEST_ENV: "preserved" },
      execute(command, args, options) {
        calls.push({ command, args, options });
        return results[Math.min(calls.length - 1, results.length - 1)];
      },
      async wait(delay) {
        waits.push(delay);
      },
      log(message) {
        logs.push(message);
      },
    },
  };
}

for (const action of ["staple", "validate"]) {
  test(`${action} retries the same artifact and recovers on the third attempt`, async () => {
    const h = harness([
      disconnected,
      disconnected,
      { status: 0, stdout: "The action worked!" },
    ]);
    await runStapler(action, "out/App with spaces.app", h.options);
    assert.equal(h.calls.length, 3);
    assert.deepEqual(h.waits, [30_000, 90_000]);
    for (const call of h.calls) {
      assert.equal(call.command, "xcrun");
      assert.deepEqual(call.args, [
        "stapler",
        action,
        "out/App with spaces.app",
      ]);
      assert.deepEqual(call.options.env, { TASK_TEST_ENV: "preserved" });
      assert.equal(call.options.timeout, 180_000);
    }
    assert.match(h.logs.join("\n"), /succeeded on attempt 3\/3/);
  });
}

test("stops at three network failures and preserves the failure exit code", async () => {
  const h = harness([disconnected]);
  await assert.rejects(
    runStapler("validate", "release.dmg", h.options),
    (error) => {
      assert.equal(error.exitCode, 68);
      assert.match(error.message, /network retry limit reached/);
      return true;
    },
  );
  assert.equal(h.calls.length, 3);
  assert.deepEqual(h.waits, [30_000, 90_000]);
});

test("returns immediately on success even if output contains a network warning", async () => {
  const h = harness([{ status: 0, stderr: "previous connection timed out" }]);
  await runStapler("validate", "release.dmg", h.options);
  assert.equal(h.calls.length, 1);
  assert.deepEqual(h.waits, []);
});

for (const output of [
  "The code signature is invalid.",
  "The ticket is invalid. The network connection was lost.",
  "No such file or directory.",
  "The validate action failed! Error 68.",
]) {
  test(`fails immediately for a non-network error: ${output}`, async () => {
    const h = harness([{ status: 65, stderr: output }]);
    await assert.rejects(
      runStapler("validate", "release.dmg", h.options),
      /not a transient network error/,
    );
    assert.equal(h.calls.length, 1);
    assert.deepEqual(h.waits, []);
  });
}

test("retries command timeouts but does not retry cancellation", async () => {
  const timeout = Object.assign(new Error("spawnSync xcrun ETIMEDOUT"), {
    code: "ETIMEDOUT",
  });
  const h = harness([
    { status: null, signal: "SIGTERM", error: timeout },
    { status: 0 },
  ]);
  await runStapler("validate", "release.dmg", h.options);
  assert.deepEqual(h.waits, [30_000]);

  const cancelled = harness([
    { ...disconnected, status: null, signal: "SIGTERM" },
  ]);
  await assert.rejects(
    runStapler("validate", "release.dmg", cancelled.options),
    /interrupted by SIGTERM/,
  );
  assert.equal(cancelled.calls.length, 1);
  assert.deepEqual(cancelled.waits, []);
});

test("fails immediately when xcrun cannot be started", async () => {
  const h = harness([
    {
      status: null,
      error: Object.assign(new Error("xcrun ENOENT"), { code: "ENOENT" }),
    },
  ]);
  await assert.rejects(
    runStapler("validate", "release.dmg", h.options),
    /xcrun ENOENT/,
  );
  assert.equal(h.calls.length, 1);
});

test("rejects unsupported commands before invoking xcrun", async () => {
  const h = harness([{ status: 0 }]);
  await assert.rejects(
    runStapler("submit", "release.dmg", h.options),
    /usage:/,
  );
  assert.equal(h.calls.length, 0);
});

test("CLI preserves artifact arguments and propagates stapler failures", () => {
  const directory = fs.mkdtempSync(
    path.join(os.tmpdir(), "csgclaw-stapler-test-"),
  );
  try {
    const captured = path.join(directory, "arguments.json");
    fs.writeFileSync(
      path.join(directory, "xcrun"),
      `#!${process.execPath}\n` +
        `require("node:fs").writeFileSync(process.env.TASK_CAPTURE, JSON.stringify(process.argv.slice(2)));\n` +
        `console.error("The ticket is invalid."); process.exit(65);\n`,
      { mode: 0o755 },
    );
    const result = spawnSync(
      process.execPath,
      [
        fileURLToPath(new URL("./macos-stapler.mjs", import.meta.url)),
        "validate",
        "out/App with spaces.app",
      ],
      {
        encoding: "utf8",
        env: {
          ...process.env,
          PATH: `${directory}${path.delimiter}${process.env.PATH}`,
          TASK_CAPTURE: captured,
        },
      },
    );
    assert.equal(result.status, 65);
    assert.deepEqual(JSON.parse(fs.readFileSync(captured, "utf8")), [
      "stapler",
      "validate",
      "out/App with spaces.app",
    ]);
    assert.match(result.stderr, /not a transient network error/);
    assert.doesNotMatch(result.stdout, /attempt 2\/3/);
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

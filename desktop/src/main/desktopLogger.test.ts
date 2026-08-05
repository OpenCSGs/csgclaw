import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  diagnosticLaunchFlags,
  initializeDesktopLogger,
  logDesktopError,
  logDesktopInfo,
} from "./desktopLogger";

test("writes structured startup diagnostics without exposing secrets", () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "csgclaw-desktop-log-"));
  try {
    const logPath = initializeDesktopLogger(directory);
    logDesktopInfo("process-start", { platform: "darwin" });
    logDesktopError("startup-failed", new Error("token=secret Bearer hidden"));

    const records = fs
      .readFileSync(logPath, "utf8")
      .trim()
      .split("\n")
      .map((line) => JSON.parse(line) as Record<string, unknown>);
    assert.equal(records[0]?.event, "process-start");
    assert.equal(records[0]?.platform, "darwin");
    assert.equal(records[0]?.runId, records[1]?.runId);
    assert.equal(records[0]?.pid, process.pid);
    assert.equal(records[1]?.errorMessage, "token=[redacted] Bearer [redacted]");
    assert.doesNotMatch(fs.readFileSync(logPath, "utf8"), /secret|hidden/);
  } finally {
    fs.rmSync(directory, { force: true, recursive: true });
  }
});

test("rotates the main process log", () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "csgclaw-desktop-log-"));
  try {
    const logPath = initializeDesktopLogger(directory, 1);
    logDesktopInfo("first");
    logDesktopInfo("second");

    assert.match(fs.readFileSync(path.join(directory, "main.previous.log"), "utf8"), /"event":"first"/);
    assert.match(fs.readFileSync(logPath, "utf8"), /"event":"second"/);
  } finally {
    fs.rmSync(directory, { force: true, recursive: true });
  }
});

test("records only command-line flag names", () => {
  assert.equal(
    diagnosticLaunchFlags(["CSGClaw", "--squirrel-firstrun", "--token=secret", "document.txt"]),
    "--squirrel-firstrun,--token",
  );
});

import assert from "node:assert/strict";
import test from "node:test";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import {
  SQUIRREL_FIRST_RUN_UPDATE_DELAY_MS,
  isSquirrelUpdateLockError,
  shouldInstallDesktopVersion,
  squirrelFirstRunUpdateDelay,
  usesMicrosoftStoreUpdates,
} from "./updatePolicy";

test("Microsoft Store packages use Store-managed updates", () => {
  assert.equal(usesMicrosoftStoreUpdates(DesktopPlatform.Windows, true), true);
});

test("non-Store Windows packages keep the configured update feed", () => {
  assert.equal(
    usesMicrosoftStoreUpdates(DesktopPlatform.Windows, false),
    false,
  );
  assert.equal(
    usesMicrosoftStoreUpdates(DesktopPlatform.Windows, undefined),
    false,
  );
});

test("non-Windows packages do not use Microsoft Store updates", () => {
  assert.equal(usesMicrosoftStoreUpdates(DesktopPlatform.MacOS, true), false);
  assert.equal(usesMicrosoftStoreUpdates(DesktopPlatform.Linux, true), false);
});

test("ordinary updates only move forward within a channel", () => {
  assert.equal(shouldInstallDesktopVersion("0.4.6", "0.5.0", false), true);
  assert.equal(shouldInstallDesktopVersion("0.5.0", "0.4.6", false), false);
  assert.equal(shouldInstallDesktopVersion("0.5.0", "0.5.0", false), false);
});

test("channel switches install the target latest regardless of version direction", () => {
  assert.equal(
    shouldInstallDesktopVersion("0.5.0-beta.5", "0.4.6", true),
    true,
  );
  assert.equal(
    shouldInstallDesktopVersion("0.4.6", "0.5.0-beta.5", true),
    true,
  );
  assert.equal(shouldInstallDesktopVersion("0.5.0", "0.5.0", true), false);
});

test("Squirrel first-run defers update checks until the installer releases its lock", () => {
  assert.equal(
    squirrelFirstRunUpdateDelay(DesktopPlatform.Windows, [
      "CSGClaw.exe",
      "--squirrel-firstrun",
    ]),
    SQUIRREL_FIRST_RUN_UPDATE_DELAY_MS,
  );
  assert.equal(
    squirrelFirstRunUpdateDelay(DesktopPlatform.Windows, ["CSGClaw.exe"]),
    0,
  );
  assert.equal(
    squirrelFirstRunUpdateDelay(DesktopPlatform.MacOS, [
      "CSGClaw",
      "--squirrel-firstrun",
    ]),
    0,
  );
});

test("recognizes the Squirrel global update lock error", () => {
  assert.equal(
    isSquirrelUpdateLockError(
      "System.Exception: Couldn't acquire lock, is another instance running",
    ),
    true,
  );
  assert.equal(isSquirrelUpdateLockError("Desktop update feed failed"), false);
});

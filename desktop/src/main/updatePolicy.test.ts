import assert from "node:assert/strict";
import test from "node:test";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import { usesMicrosoftStoreUpdates } from "./updatePolicy";

test("Microsoft Store packages use Store-managed updates", () => {
  assert.equal(usesMicrosoftStoreUpdates(DesktopPlatform.Windows, true), true);
});

test("non-Store Windows packages keep the configured update feed", () => {
  assert.equal(usesMicrosoftStoreUpdates(DesktopPlatform.Windows, false), false);
  assert.equal(usesMicrosoftStoreUpdates(DesktopPlatform.Windows, undefined), false);
});

test("non-Windows packages do not use Microsoft Store updates", () => {
  assert.equal(usesMicrosoftStoreUpdates(DesktopPlatform.MacOS, true), false);
  assert.equal(usesMicrosoftStoreUpdates(DesktopPlatform.Linux, true), false);
});

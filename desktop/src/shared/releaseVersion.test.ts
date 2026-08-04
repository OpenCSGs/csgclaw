import assert from "node:assert/strict";
import test from "node:test";

import { normalizeDesktopReleaseVersion, numericDesktopAppVersion } from "./releaseVersion";

test("preserves stable and prerelease desktop versions", () => {
  assert.equal(normalizeDesktopReleaseVersion("v0.4.5"), "0.4.5");
  assert.equal(normalizeDesktopReleaseVersion("0.4.5-beta.1"), "0.4.5-beta.1");
});

test("drops build metadata that packaging formats do not need", () => {
  assert.equal(normalizeDesktopReleaseVersion("v0.4.5-beta.1+local"), "0.4.5-beta.1");
});

test("uses the development version for invalid input", () => {
  assert.equal(normalizeDesktopReleaseVersion("not-a-version"), "0.0.0-development");
  assert.equal(normalizeDesktopReleaseVersion(undefined), "0.0.0-development");
});

test("uses a numeric system app version for prerelease packages", () => {
  assert.equal(numericDesktopAppVersion("0.4.5-beta.1"), "0.4.5");
  assert.equal(numericDesktopAppVersion("invalid"), "0.0.0");
});

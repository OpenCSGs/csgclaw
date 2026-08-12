import assert from "node:assert/strict";
import test from "node:test";

import {
  compareDesktopReleaseVersions,
  normalizeDesktopReleaseVersion,
  numericDesktopAppVersion,
} from "./releaseVersion";

test("preserves stable and prerelease desktop versions", () => {
  assert.equal(normalizeDesktopReleaseVersion("v0.4.5"), "0.4.5");
  assert.equal(normalizeDesktopReleaseVersion("0.4.5-beta.1"), "0.4.5-beta.1");
});

test("drops build metadata that packaging formats do not need", () => {
  assert.equal(
    normalizeDesktopReleaseVersion("v0.4.5-beta.1+local"),
    "0.4.5-beta.1",
  );
});

test("normalizes local git describe versions for Squirrel", () => {
  assert.equal(
    normalizeDesktopReleaseVersion("v0.4.5-12-g27f214c7+local"),
    "0.4.5-dev12g27f214c7",
  );
  assert.equal(
    normalizeDesktopReleaseVersion("v0.4.5-test6-12-g27f214c7+local"),
    "0.4.5-test6dev12g27f214c7",
  );
});

test("uses the development version for invalid input", () => {
  assert.equal(
    normalizeDesktopReleaseVersion("not-a-version"),
    "0.0.0-development",
  );
  assert.equal(normalizeDesktopReleaseVersion(undefined), "0.0.0-development");
});

test("uses a numeric system app version for prerelease packages", () => {
  assert.equal(numericDesktopAppVersion("0.4.5-beta.1"), "0.4.5");
  assert.equal(numericDesktopAppVersion("invalid"), "0.0.0");
});

test("compares stable and prerelease desktop versions using SemVer precedence", () => {
  assert.equal(
    compareDesktopReleaseVersions("0.5.0-beta.2", "0.5.0-beta.3"),
    -1,
  );
  assert.equal(
    compareDesktopReleaseVersions("v0.5.0-beta.3", "0.5.0-beta.2"),
    1,
  );
  assert.equal(compareDesktopReleaseVersions("0.5.0-beta.3", "0.5.0"), -1);
  assert.equal(compareDesktopReleaseVersions("0.5.0", "0.5.0-beta.3"), 1);
  assert.equal(
    compareDesktopReleaseVersions("0.5.0+build.1", "0.5.0+build.2"),
    0,
  );
  assert.equal(
    compareDesktopReleaseVersions("0.5.0-beta.10", "0.5.0-beta.2"),
    1,
  );
});

test("rejects invalid desktop versions during comparison", () => {
  assert.throws(
    () => compareDesktopReleaseVersions("dev", "0.5.0"),
    /invalid desktop versions/,
  );
});

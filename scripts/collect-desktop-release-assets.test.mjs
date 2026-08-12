import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { collectDesktopReleaseAssets } from "./collect-desktop-release-assets.mjs";

const version = "v0.4.3";

test("collects the public asset set for each supported target", async (t) => {
  for (const fixture of [
    { goos: "darwin", goarch: "arm64", files: ["dmg/CSGClaw-0.4.3-arm64.dmg", "zip/darwin/arm64/CSGClaw-darwin-arm64-0.4.3.zip"], want: [".dmg", ".zip"] },
    { goos: "darwin", goarch: "amd64", files: ["dmg/CSGClaw-0.4.3-x64.dmg", "zip/darwin/x64/CSGClaw-darwin-x64-0.4.3.zip"], want: [".dmg", ".zip"] },
    { goos: "windows", goarch: "amd64", files: ["squirrel.windows/x64/CSGClaw-Desktop-0.4.3-x64-Setup.exe"], want: [".exe"] },
    { goos: "linux", goarch: "amd64", files: ["deb/x64/csgclaw.deb"], want: [".deb"] },
    { goos: "linux", goarch: "arm64", files: ["deb/arm64/csgclaw.deb"], want: [".deb"] },
  ]) {
    await t.test(`${fixture.goos}/${fixture.goarch}`, () => {
      const { makeDirectory, outputDirectory, cleanup } = fixtureDirectories(fixture.files);
      try {
        collectDesktopReleaseAssets({ version, ...fixture, makeDirectory, outputDirectory });
        assert.deepEqual(
          fs.readdirSync(outputDirectory).sort(),
          fixture.want.map((extension) => `csgclaw-desktop_${version}_${fixture.goos}_${fixture.goarch}${extension}`),
        );
      } finally {
        cleanup();
      }
    });
  }
});

test("rejects a missing expected asset", () => {
  const { makeDirectory, outputDirectory, cleanup } = fixtureDirectories(["CSGClaw-0.4.3-arm64.dmg"]);
  try {
    assert.throws(
      () => collectDesktopReleaseAssets({ version, goos: "darwin", goarch: "arm64", makeDirectory, outputDirectory }),
      /expected exactly one source/,
    );
  } finally {
    cleanup();
  }
});

test("rejects ambiguous Forge output", () => {
  const { makeDirectory, outputDirectory, cleanup } = fixtureDirectories(["first.deb", "second.deb"]);
  try {
    assert.throws(
      () => collectDesktopReleaseAssets({ version, goos: "linux", goarch: "amd64", makeDirectory, outputDirectory }),
      /expected exactly one source/,
    );
  } finally {
    cleanup();
  }
});

test("ignores stale artifacts for another version", () => {
  const { makeDirectory, outputDirectory, cleanup } = fixtureDirectories([
    "dmg/CSGClaw-0.4.2-arm64.dmg",
    "dmg/CSGClaw-0.4.3-arm64.dmg",
    "zip/darwin/arm64/CSGClaw-darwin-arm64-0.4.2.zip",
    "zip/darwin/arm64/CSGClaw-darwin-arm64-0.4.3.zip",
  ]);
  try {
    collectDesktopReleaseAssets({ version, goos: "darwin", goarch: "arm64", makeDirectory, outputDirectory });
    assert.deepEqual(
      fs.readdirSync(outputDirectory).sort(),
      [
        "csgclaw-desktop_v0.4.3_darwin_arm64.dmg",
        "csgclaw-desktop_v0.4.3_darwin_arm64.zip",
      ],
    );
  } finally {
    cleanup();
  }
});

test("uses the same canonical asset name for versions with or without v", () => {
  const { makeDirectory, outputDirectory, cleanup } = fixtureDirectories(["deb/x64/csgclaw.deb"]);
  try {
    collectDesktopReleaseAssets({
      version: "0.4.3",
      goos: "linux",
      goarch: "amd64",
      makeDirectory,
      outputDirectory,
    });
    assert.deepEqual(fs.readdirSync(outputDirectory), ["csgclaw-desktop_v0.4.3_linux_amd64.deb"]);
  } finally {
    cleanup();
  }
});

test("stages Forge update feeds without changing the public installer names", async (t) => {
  for (const fixture of [
    {
      name: "macOS",
      goos: "darwin",
      goarch: "arm64",
      files: [
        "dmg/CSGClaw-0.4.3-arm64.dmg",
        "zip/darwin/arm64/CSGClaw-darwin-arm64-0.4.3.zip",
        "zip/darwin/arm64/RELEASES.json",
      ],
      feed: ["CSGClaw-darwin-arm64-0.4.3.zip", "RELEASES.json"],
      platform: "darwin",
      arch: "arm64",
    },
    {
      name: "Windows",
      goos: "windows",
      goarch: "amd64",
      files: [
        "squirrel.windows/x64/CSGClaw-Desktop-0.4.3-x64-Setup.exe",
        "squirrel.windows/x64/csgclaw_desktop-0.4.3-full.nupkg",
        "squirrel.windows/x64/RELEASES",
      ],
      feed: ["RELEASES", "csgclaw_desktop-0.4.3-full.nupkg"],
      platform: "win32",
      arch: "x64",
    },
  ]) {
    await t.test(fixture.name, () => {
      const { makeDirectory, outputDirectory, cleanup } = fixtureDirectories(fixture.files);
      try {
        collectDesktopReleaseAssets({ version, ...fixture, makeDirectory, outputDirectory });
        assert.deepEqual(
          fs.readdirSync(path.join(outputDirectory, "updates", fixture.platform, fixture.arch)).sort(),
          fixture.feed,
        );
      } finally {
        cleanup();
      }
    });
  }
});

test("rejects a partial desktop update feed", () => {
  const { makeDirectory, outputDirectory, cleanup } = fixtureDirectories([
    "squirrel.windows/x64/CSGClaw-Desktop-0.4.3-x64-Setup.exe",
    "squirrel.windows/x64/RELEASES",
  ]);
  try {
    assert.throws(
      () => collectDesktopReleaseAssets({ version, goos: "windows", goarch: "amd64", makeDirectory, outputDirectory }),
      /incomplete Windows update feed/,
    );
  } finally {
    cleanup();
  }
});

function fixtureDirectories(files) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "csgclaw-desktop-assets-"));
  const makeDirectory = path.join(root, "make");
  const outputDirectory = path.join(root, "output");
  fs.mkdirSync(makeDirectory, { recursive: true });
  for (const file of files) {
    const fixturePath = path.join(makeDirectory, file);
    fs.mkdirSync(path.dirname(fixturePath), { recursive: true });
    fs.writeFileSync(fixturePath, "fixture");
  }
  return {
    makeDirectory,
    outputDirectory,
    cleanup: () => fs.rmSync(root, { recursive: true, force: true }),
  };
}

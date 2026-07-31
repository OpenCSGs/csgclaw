import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { collectDesktopReleaseAssets } from "./collect-desktop-release-assets.mjs";

const version = "v0.4.3";

test("collects the public asset set for each supported target", async (t) => {
  for (const fixture of [
    { goos: "darwin", goarch: "arm64", files: ["dmg/CSGClaw.dmg", "zip/darwin/arm64/CSGClaw.zip"], want: [".dmg", ".zip"] },
    { goos: "darwin", goarch: "amd64", files: ["dmg/CSGClaw.dmg", "zip/darwin/x64/CSGClaw.zip"], want: [".dmg", ".zip"] },
    { goos: "windows", goarch: "amd64", files: ["squirrel.windows/x64/CSGClaw-Setup.exe"], want: [".exe"] },
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
  const { makeDirectory, outputDirectory, cleanup } = fixtureDirectories(["CSGClaw.dmg"]);
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

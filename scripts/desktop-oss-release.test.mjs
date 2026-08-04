import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  desktopUploadPaths,
  generateDownloadsManifest,
  inferReleaseChannel,
  normalizeReleaseVersion,
  releaseTag,
  validateReleaseChannel,
} from "./desktop-oss-release.mjs";

test("normalizes public desktop versions", () => {
  assert.equal(normalizeReleaseVersion("v0.4.5-beta.1"), "0.4.5-beta.1");
  assert.equal(normalizeReleaseVersion("0.4.5+local"), "0.4.5");
  assert.equal(releaseTag("v0.4.5-beta.1"), "v0.4.5-beta.1");
  assert.equal(inferReleaseChannel("0.4.5-beta.1"), "beta");
  assert.equal(inferReleaseChannel("0.4.5"), "release");
  assert.equal(inferReleaseChannel("v0.4.6-beta.1"), "beta");
  assert.equal(inferReleaseChannel("v0.4.6"), "release");
  assert.throws(() => inferReleaseChannel("v0.4.6.beta.1"), /invalid release version/);
});

test("keeps beta and release versions in their channels", () => {
  assert.doesNotThrow(() => validateReleaseChannel("0.4.5-beta.1", "beta"));
  assert.doesNotThrow(() => validateReleaseChannel("0.4.5", "release"));
  assert.throws(() => validateReleaseChannel("0.4.5-beta.1", "release"), /cannot publish/);
});

test("selects only website installers for OSS upload", () => {
  assert.deepEqual(
    desktopUploadPaths("0.4.5-beta.1", "/release").map((file) => path.basename(file)),
    [
      "csgclaw-desktop_v0.4.5-beta.1_darwin_arm64.dmg",
      "csgclaw-desktop_v0.4.5-beta.1_darwin_amd64.dmg",
      "csgclaw-desktop_v0.4.5-beta.1_windows_amd64.exe",
    ],
  );
});

test("generates a downloads manifest compatible with the existing csglite schema", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "csgclaw-desktop-manifest-"));
  const version = "0.4.5-beta.1";
  const releaseDirectory = path.join(root, "releases", version);
  const manifestPath = path.join(root, "channels", "beta", "downloads.json");
  fs.mkdirSync(releaseDirectory, { recursive: true });
  for (const suffix of ["darwin_arm64.dmg", "darwin_amd64.dmg", "windows_amd64.exe"]) {
    fs.writeFileSync(path.join(releaseDirectory, `csgclaw-desktop_v${version}_${suffix}`), suffix);
  }

  try {
    const manifest = generateDownloadsManifest({
      version,
      channel: "beta",
      releaseDirectory,
      manifestPath,
      publicBaseURL: "https://downloads.example/csgclaw-desktop",
      publishedAt: "2026-08-04T00:00:00.000Z",
    });
    assert.equal(manifest.latest, version);
    assert.deepEqual(
      manifest.versions[version].artifacts.map(({ platform, arch }) => ({ platform, arch })),
      [
        { platform: "macos", arch: "arm64" },
        { platform: "macos", arch: "x86_64" },
        { platform: "windows", arch: "x86_64" },
      ],
    );
    assert.match(manifest.versions[version].artifacts[0].sha256, /^[a-f0-9]{64}$/);
    assert.match(manifest.versions[version].artifacts[0].url, /csgclaw-desktop_v0\.4\.5-beta\.1_darwin_arm64\.dmg$/);
    assert.equal(JSON.parse(fs.readFileSync(manifestPath, "utf8")).channel, "beta");
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

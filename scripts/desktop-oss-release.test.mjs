import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  compareReleaseVersions,
  desktopUpdateFeedPaths,
  desktopUploadPaths,
  generateDownloadsManifest,
  inferReleaseChannel,
  legacyMacUpdateManifestRelativePath,
  normalizeReleaseVersion,
  releasePackagePaths,
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
  assert.throws(
    () => inferReleaseChannel("v0.4.6.beta.1"),
    /invalid release version/,
  );
  assert.throws(
    () => inferReleaseChannel("v0.4.6-beta.01"),
    /invalid release version/,
  );
});

test("orders public desktop versions using SemVer precedence", () => {
  assert.equal(compareReleaseVersions("v0.4.6", "0.4.6+build.2"), 0);
  assert.ok(compareReleaseVersions("0.4.6", "0.4.6-rc.1") > 0);
  assert.ok(compareReleaseVersions("0.4.6-beta.10", "0.4.6-beta.2") > 0);
  assert.ok(compareReleaseVersions("0.4.6-beta.2", "0.4.6-beta.2.1") < 0);
  assert.ok(compareReleaseVersions("0.4.5", "0.4.6") < 0);
});

test("keeps beta and release versions in their channels", () => {
  assert.doesNotThrow(() => validateReleaseChannel("0.4.5-beta.1", "beta"));
  assert.doesNotThrow(() => validateReleaseChannel("0.4.5", "release"));
  assert.throws(
    () => validateReleaseChannel("0.4.5-beta.1", "release"),
    /cannot publish/,
  );
});

test("selects only website installers for OSS upload", () => {
  assert.deepEqual(
    desktopUploadPaths("0.4.5-beta.1", "/release").map((file) =>
      path.basename(file),
    ),
    [
      "csgclaw-desktop_v0.4.5-beta.1_darwin_arm64.dmg",
      "csgclaw-desktop_v0.4.5-beta.1_darwin_amd64.dmg",
      "csgclaw-desktop_v0.4.5-beta.1_windows_amd64.exe",
    ],
  );
});

test("generates a downloads manifest compatible with the existing csglite schema", () => {
  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "csgclaw-desktop-manifest-"),
  );
  const version = "0.4.5-beta.1";
  const releaseDirectory = path.join(root, "releases", version);
  const manifestPath = path.join(root, "channels", "beta", "downloads.json");
  fs.mkdirSync(releaseDirectory, { recursive: true });
  for (const suffix of [
    "darwin_arm64.dmg",
    "darwin_amd64.dmg",
    "windows_amd64.exe",
  ]) {
    fs.writeFileSync(
      path.join(releaseDirectory, `csgclaw-desktop_v${version}_${suffix}`),
      suffix,
    );
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
      manifest.versions[version].artifacts.map(({ platform, arch }) => ({
        platform,
        arch,
      })),
      [
        { platform: "macos", arch: "arm64" },
        { platform: "macos", arch: "x86_64" },
        { platform: "windows", arch: "x86_64" },
      ],
    );
    assert.match(
      manifest.versions[version].artifacts[0].sha256,
      /^[a-f0-9]{64}$/,
    );
    assert.match(
      manifest.versions[version].artifacts[0].url,
      /csgclaw-desktop_v0\.4\.5-beta\.1_darwin_arm64\.dmg$/,
    );
    assert.equal(
      JSON.parse(fs.readFileSync(manifestPath, "utf8")).channel,
      "beta",
    );

    const repeated = generateDownloadsManifest({
      version,
      channel: "beta",
      releaseDirectory,
      manifestPath,
      publishedAt: "2026-08-05T00:00:00.000Z",
    });
    assert.equal(repeated.latest, version);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("adds GitLab-compatible server and CLI archives without changing desktop artifacts", () => {
  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "csgclaw-release-packages-"),
  );
  const version = "0.4.6";
  const releaseDirectory = path.join(root, "releases", version);
  const manifestPath = path.join(root, "channels", "release", "downloads.json");
  fs.mkdirSync(releaseDirectory, { recursive: true });
  for (const suffix of [
    "darwin_arm64.dmg",
    "darwin_amd64.dmg",
    "windows_amd64.exe",
  ]) {
    fs.writeFileSync(
      path.join(releaseDirectory, `csgclaw-desktop_v${version}_${suffix}`),
      suffix,
    );
  }
  for (const osArch of [
    ["linux", "amd64"],
    ["linux", "arm64"],
    ["darwin", "arm64"],
    ["darwin", "amd64"],
    ["windows", "amd64"],
  ]) {
    const [osName, arch] = osArch;
    const extension = osName === "windows" ? "zip" : "tar.gz";
    for (const app of ["csgclaw", "csgclaw-cli"]) {
      fs.writeFileSync(
        path.join(
          releaseDirectory,
          `${app}_v${version}_${osName}_${arch}.${extension}`,
        ),
        `${app}-${osName}-${arch}`,
      );
    }
  }

  try {
    const manifest = generateDownloadsManifest({
      version,
      channel: "release",
      releaseDirectory,
      manifestPath,
      publicBaseURL: "https://downloads.example/csgclaw-desktop",
      requirePackages: true,
    });
    assert.equal(manifest.versions[version].artifacts.length, 3);
    assert.equal(manifest.versions[version].packages.length, 10);
    assert.deepEqual(manifest.versions[version].packages[0], {
      kind: "server",
      os: "linux",
      arch: "amd64",
      name: "csgclaw_v0.4.6_linux_amd64.tar.gz",
      url: "https://downloads.example/csgclaw-desktop/releases/0.4.6/csgclaw_v0.4.6_linux_amd64.tar.gz",
      size_bytes: Buffer.byteLength("csgclaw-linux-amd64"),
      sha256: manifest.versions[version].packages[0].sha256,
    });
    assert.match(
      manifest.versions[version].packages[0].sha256,
      /^[a-f0-9]{64}$/,
    );
    assert.equal(releasePackagePaths(version, releaseDirectory).length, 10);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("requires the complete Server and CLI package matrix when requested", () => {
  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "csgclaw-release-packages-missing-"),
  );
  const version = "0.4.6";
  const releaseDirectory = path.join(root, "releases", version);
  const manifestPath = path.join(root, "channels", "release", "downloads.json");
  fs.mkdirSync(releaseDirectory, { recursive: true });
  for (const suffix of [
    "darwin_arm64.dmg",
    "darwin_amd64.dmg",
    "windows_amd64.exe",
  ]) {
    fs.writeFileSync(
      path.join(releaseDirectory, `csgclaw-desktop_v${version}_${suffix}`),
      suffix,
    );
  }

  try {
    assert.throws(
      () =>
        generateDownloadsManifest({
          version,
          channel: "release",
          releaseDirectory,
          manifestPath,
          requirePackages: true,
        }),
      /cannot generate complete release package set/,
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("requires complete native Electron update feeds before publishing", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "csgclaw-update-feeds-"));
  const files = [
    "updates/darwin/arm64/CSGClaw-darwin-arm64-0.4.6.zip",
    "updates/darwin/arm64/RELEASES.json",
    "updates/darwin/x64/CSGClaw-darwin-x64-0.4.6.zip",
    "updates/darwin/x64/RELEASES.json",
    "updates/win32/x64/csgclaw_desktop-0.4.6-full.nupkg",
    "updates/win32/x64/RELEASES",
  ];
  for (const relativePath of files) {
    const filePath = path.join(root, relativePath);
    fs.mkdirSync(path.dirname(filePath), { recursive: true });
    fs.writeFileSync(filePath, relativePath);
  }

  try {
    assert.deepEqual(
      desktopUpdateFeedPaths(root)
        .map((filePath) => path.relative(root, filePath))
        .sort(),
      files.sort(),
    );
    fs.rmSync(path.join(root, "updates", "win32", "x64", "RELEASES"));
    assert.throws(
      () => desktopUpdateFeedPaths(root),
      /desktop update feed is incomplete/,
    );
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("publishes legacy macOS update manifest aliases for installed clients", () => {
  assert.equal(
    legacyMacUpdateManifestRelativePath("darwin/arm64/RELEASES.json"),
    "darwin/arm64",
  );
  assert.equal(
    legacyMacUpdateManifestRelativePath("darwin/x64/RELEASES.json"),
    "darwin/x64",
  );
  assert.equal(legacyMacUpdateManifestRelativePath("win32/x64/RELEASES"), "");
  assert.equal(
    legacyMacUpdateManifestRelativePath("darwin/arm64/archive.zip"),
    "",
  );
});

test("rejects moving a channel latest backward", () => {
  const root = fs.mkdtempSync(
    path.join(os.tmpdir(), "csgclaw-desktop-rollback-"),
  );
  const version = "0.4.5-beta.1";
  const currentLatest = "0.4.5-beta.2";
  const releaseDirectory = path.join(root, "releases", version);
  const manifestPath = path.join(root, "channels", "beta", "downloads.json");
  fs.mkdirSync(releaseDirectory, { recursive: true });
  for (const suffix of [
    "darwin_arm64.dmg",
    "darwin_amd64.dmg",
    "windows_amd64.exe",
  ]) {
    fs.writeFileSync(
      path.join(releaseDirectory, `csgclaw-desktop_v${version}_${suffix}`),
      suffix,
    );
  }
  const existingManifest = `${JSON.stringify(
    {
      schema_version: 1,
      channel: "beta",
      latest: currentLatest,
      versions: {
        [currentLatest]: {
          version: currentLatest,
          published_at: "2026-08-04T00:00:00.000Z",
          artifacts: [],
        },
      },
    },
    null,
    2,
  )}\n`;
  fs.mkdirSync(path.dirname(manifestPath), { recursive: true });
  fs.writeFileSync(manifestPath, existingManifest);

  try {
    assert.throws(
      () =>
        generateDownloadsManifest({
          version,
          channel: "beta",
          releaseDirectory,
          manifestPath,
        }),
      /refusing to publish 0\.4\.5-beta\.1 to beta: current latest is newer \(0\.4\.5-beta\.2\)/,
    );
    assert.equal(fs.readFileSync(manifestPath, "utf8"), existingManifest);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

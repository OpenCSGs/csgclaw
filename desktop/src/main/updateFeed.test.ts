import assert from "node:assert/strict";
import test from "node:test";
import {
  desktopDownloadsManifestURL,
  desktopInstallerArtifact,
  desktopUpdateFeedOptions,
  desktopUpdateFeedURL,
  normalizeHTTPSURL,
  parseDesktopDownloadsManifest,
} from "./updateFeed";

test("builds isolated release and beta OSS update feeds", () => {
  const base = "https://downloads.example/csgclaw/channels";
  assert.equal(
    desktopUpdateFeedURL({
      channel: "release",
      platform: "darwin",
      arch: "arm64",
      channelsBaseURL: base,
    }),
    "https://downloads.example/csgclaw/channels/release/updates/darwin/arm64/RELEASES.json",
  );
  assert.equal(
    desktopUpdateFeedURL({
      channel: "beta",
      platform: "win32",
      arch: "x64",
      channelsBaseURL: base,
    }),
    "https://downloads.example/csgclaw/channels/beta/updates/win32/x64",
  );
});

test("builds the channel manifest URL used before native update checks", () => {
  assert.equal(
    desktopDownloadsManifestURL({
      channel: "beta",
      channelsBaseURL: "https://downloads.example/csgclaw/channels/",
    }),
    "https://downloads.example/csgclaw/channels/beta/downloads.json",
  );
});

test("uses Electron's JSON server mode for macOS prerelease comparisons", () => {
  assert.deepEqual(
    desktopUpdateFeedOptions("https://updates.example/RELEASES.json", "darwin"),
    {
      url: "https://updates.example/RELEASES.json",
      serverType: "json",
    },
  );
  assert.deepEqual(
    desktopUpdateFeedOptions("https://updates.example/windows", "win32"),
    {
      url: "https://updates.example/windows",
    },
  );
});

test("validates the selected channel and latest manifest entry", () => {
  assert.deepEqual(
    parseDesktopDownloadsManifest(
      {
        schema_version: 1,
        channel: "beta",
        latest: "v0.5.0-beta.3",
        versions: {
          "0.5.0-beta.3": {
            version: "0.5.0-beta.3",
            artifacts: [
              {
                platform: "macos",
                arch: "arm64",
                url: "https://downloads.example/csgclaw/beta3.dmg",
                size_bytes: 123,
                sha256: "a".repeat(64),
              },
            ],
          },
        },
      },
      "beta",
    ),
    {
      channel: "beta",
      latest: "0.5.0-beta.3",
      artifacts: [
        {
          platform: "macos",
          arch: "arm64",
          url: "https://downloads.example/csgclaw/beta3.dmg",
          sizeBytes: 123,
          sha256: "a".repeat(64),
        },
      ],
    },
  );
  assert.throws(
    () =>
      parseDesktopDownloadsManifest(
        {
          schema_version: 1,
          channel: "release",
          latest: "0.5.0",
          versions: { "0.5.0": {} },
        },
        "beta",
      ),
    /expected beta/,
  );
  assert.throws(
    () =>
      parseDesktopDownloadsManifest(
        { schema_version: 1, channel: "beta", latest: "0.5.0", versions: {} },
        "beta",
      ),
    /missing from versions/,
  );
});

test("selects the complete installer for the running platform and architecture", () => {
  const manifest = parseDesktopDownloadsManifest(
    {
      schema_version: 1,
      channel: "release",
      latest: "0.4.6",
      versions: {
        "0.4.6": {
          artifacts: [
            {
              platform: "windows",
              arch: "x86_64",
              url: "https://downloads.example/csgclaw/release.exe",
              size_bytes: 456,
              sha256: "b".repeat(64),
            },
          ],
        },
      },
    },
    "release",
  );
  assert.deepEqual(desktopInstallerArtifact(manifest, "win32", "x64"), {
    platform: "windows",
    arch: "x86_64",
    url: "https://downloads.example/csgclaw/release.exe",
    sizeBytes: 456,
    sha256: "b".repeat(64),
  });
  assert.equal(
    desktopInstallerArtifact(manifest, "darwin", "arm64"),
    undefined,
  );
});

test("allows a direct feed override and rejects unsafe URLs", () => {
  assert.equal(
    desktopUpdateFeedURL({
      channel: "beta",
      platform: "darwin",
      arch: "arm64",
      directURL: "https://updates.example/custom/",
    }),
    "https://updates.example/custom",
  );
  assert.equal(normalizeHTTPSURL("http://updates.example/feed"), "");
  assert.equal(
    normalizeHTTPSURL("https://user:secret@updates.example/feed"),
    "",
  );
});

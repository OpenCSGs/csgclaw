import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  compareReleaseVersions,
  desktopPackageDownloadURLs,
  desktopPackageUploadPaths,
  desktopUpdateFeedPaths,
  desktopUploadPaths,
  fetchDownloadsManifest,
  formatReleaseError,
  generateDownloadsManifest,
  inferReleaseChannel,
  legacyMacUpdateManifestRelativePath,
  normalizeReleaseVersion,
  releasePackagePaths,
  releaseTag,
  validateReleaseChannel,
} from "./desktop-oss-release.mjs";
import { validateManualAlphaVersion } from "./desktop-release-artifacts.mjs";

const manifestURL = "https://downloads.example/channels/beta/downloads.json";

test("retries manifest connection failures with backoff and fresh timeouts", async (t) => {
  const networkError = new TypeError("fetch failed", {
    cause: Object.assign(new Error("connection reset"), { code: "ECONNRESET" }),
  });
  const signals = [];
  const fetchMock = t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(url, manifestURL);
    assert.equal(options.cache, "no-store");
    assert.ok(options.signal instanceof AbortSignal);
    signals.push(options.signal);
    if (signals.length < 3) {
      throw networkError;
    }
    return Response.json({ latest: "0.8.0-beta.4" });
  });
  const warnings = t.mock.method(console, "warn", () => {});

  assert.deepEqual(
    await fetchDownloadsManifest(manifestURL, { retryDelayMS: 1 }),
    { latest: "0.8.0-beta.4" },
  );
  assert.equal(fetchMock.mock.callCount(), 3);
  assert.equal(new Set(signals).size, 3);
  assert.equal(warnings.mock.callCount(), 2);
  assert.match(warnings.mock.calls[0].arguments[0], /ECONNRESET/);
  assert.match(
    warnings.mock.calls[0].arguments[0],
    /attempt 1\/3.*retrying in 1ms/s,
  );
  assert.match(
    warnings.mock.calls[1].arguments[0],
    /attempt 2\/3.*retrying in 2ms/s,
  );
});

test("stops after three manifest failures and retains the underlying cause", async (t) => {
  const networkError = new TypeError("fetch failed", {
    cause: Object.assign(new Error("getaddrinfo ENOTFOUND downloads.example"), {
      code: "ENOTFOUND",
    }),
  });
  const fetchMock = t.mock.method(globalThis, "fetch", async () => {
    throw networkError;
  });
  t.mock.method(console, "warn", () => {});

  await assert.rejects(
    fetchDownloadsManifest(manifestURL, { retryDelayMS: 0 }),
    (error) => {
      assert.equal(error.cause, networkError);
      const diagnostic = formatReleaseError(error);
      assert.ok(diagnostic.includes(manifestURL));
      assert.match(diagnostic, /attempt 3\/3/);
      assert.match(diagnostic, /TypeError: fetch failed/);
      assert.match(diagnostic, /ENOTFOUND/);
      return true;
    },
  );
  assert.equal(fetchMock.mock.callCount(), 3);
});

test("retries transient HTTP errors while reading a manifest", async (t) => {
  for (const status of [408, 429, 500, 503]) {
    await t.test(`HTTP ${status}`, async (t) => {
      let calls = 0;
      t.mock.method(globalThis, "fetch", async () => {
        calls += 1;
        return calls === 1
          ? new Response("temporary failure", { status })
          : Response.json({ latest: "0.8.0-beta.4" });
      });
      t.mock.method(console, "warn", () => {});

      assert.deepEqual(
        await fetchDownloadsManifest(manifestURL, { retryDelayMS: 0 }),
        { latest: "0.8.0-beta.4" },
      );
      assert.equal(calls, 2);
    });
  }
});

test("fails immediately on permanent HTTP errors and invalid JSON", async (t) => {
  for (const [name, response, expected] of [
    ["forbidden", new Response("forbidden", { status: 403 }), /HTTP 403/],
    [
      "missing verification manifest",
      new Response("missing", { status: 404 }),
      /HTTP 404/,
    ],
    ["invalid JSON", new Response("invalid JSON"), /SyntaxError/],
  ]) {
    await t.test(name, async (t) => {
      const fetchMock = t.mock.method(
        globalThis,
        "fetch",
        async () => response,
      );
      const warnings = t.mock.method(console, "warn", () => {});

      await assert.rejects(fetchDownloadsManifest(manifestURL), (error) => {
        assert.match(formatReleaseError(error), expected);
        return true;
      });
      assert.equal(fetchMock.mock.callCount(), 1);
      assert.equal(warnings.mock.callCount(), 0);
    });
  }
});

test("allows a missing manifest only when reading a channel before publishing", async (t) => {
  const fetchMock = t.mock.method(
    globalThis,
    "fetch",
    async () => new Response("missing", { status: 404 }),
  );
  assert.equal(
    await fetchDownloadsManifest(manifestURL, { allowNotFound: true }),
    undefined,
  );
  assert.equal(fetchMock.mock.callCount(), 1);
});

test("retries connections lost while consuming the manifest body", async (t) => {
  let calls = 0;
  t.mock.method(globalThis, "fetch", async () => {
    calls += 1;
    if (calls === 1) {
      return new Response(
        new ReadableStream({
          start(controller) {
            controller.error(
              new TypeError("terminated", {
                cause: Object.assign(new Error("other side closed"), {
                  code: "UND_ERR_SOCKET",
                }),
              }),
            );
          },
        }),
      );
    }
    return Response.json({ latest: "0.8.0-beta.4" });
  });
  t.mock.method(console, "warn", () => {});

  assert.deepEqual(
    await fetchDownloadsManifest(manifestURL, { retryDelayMS: 0 }),
    { latest: "0.8.0-beta.4" },
  );
  assert.equal(calls, 2);
});

test("bounds and retries stalled requests and body reads", async (t) => {
  for (const phase of ["request", "body"]) {
    await t.test(phase, async (t) => {
      const fetchMock = t.mock.method(
        globalThis,
        "fetch",
        async (_url, { signal }) => {
          const stalled = () =>
            new Promise((_resolve, reject) => {
              const watchdog = setTimeout(
                () => reject(new Error("request did not abort")),
                1_000,
              );
              signal.addEventListener(
                "abort",
                () => {
                  clearTimeout(watchdog);
                  reject(signal.reason);
                },
                { once: true },
              );
            });
          return phase === "request" ? stalled() : { ok: true, json: stalled };
        },
      );
      t.mock.method(console, "warn", () => {});

      await assert.rejects(
        fetchDownloadsManifest(manifestURL, { timeoutMS: 5, retryDelayMS: 0 }),
        (error) => {
          assert.equal(error.cause.name, "TimeoutError");
          assert.match(error.message, /attempt 3\/3/);
          return true;
        },
      );
      assert.equal(fetchMock.mock.callCount(), 3);
    });
  }
});

test("reports nested and aggregate network causes without dumping arbitrary properties", () => {
  const connectionErrors = [
    Object.assign(new Error("connect timed out"), { code: "ETIMEDOUT" }),
    Object.assign(new Error("certificate expired"), {
      code: "CERT_HAS_EXPIRED",
    }),
  ];
  const error = new TypeError("fetch failed", {
    cause: new AggregateError(connectionErrors, "all connections failed"),
  });
  error.environment = { OSS_ACCESS_KEY_SECRET: "must-not-be-logged" };
  const diagnostic = formatReleaseError(error);
  assert.match(diagnostic, /AggregateError: all connections failed/);
  assert.match(diagnostic, /ETIMEDOUT/);
  assert.match(diagnostic, /CERT_HAS_EXPIRED/);
  assert.ok(!diagnostic.includes("must-not-be-logged"));
  error.cause = error;
  assert.match(formatReleaseError(error), /circular error/);
  assert.equal(formatReleaseError("unexpected failure"), "unexpected failure");
});

test("normalizes public desktop versions", () => {
  assert.equal(normalizeReleaseVersion("v0.4.5-beta.1"), "0.4.5-beta.1");
  assert.equal(normalizeReleaseVersion("0.4.5+local"), "0.4.5");
  assert.equal(releaseTag("v0.4.5-beta.1"), "v0.4.5-beta.1");
  assert.equal(inferReleaseChannel("0.4.5-beta.1"), "beta");
  assert.equal(inferReleaseChannel("0.4.5"), "release");
  assert.equal(inferReleaseChannel("v0.4.6-beta.1"), "beta");
  assert.equal(inferReleaseChannel("v0.4.6-alpha.1"), "beta");
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

test("manual workflow accepts only numbered alpha versions with a v prefix", () => {
  assert.equal(validateManualAlphaVersion("v0.2.1-alpha.1"), "0.2.1-alpha.1");
  for (const version of [
    "0.2.1-alpha.1",
    "v0.2.1-alpha",
    "v0.2.1-alpha.01",
    "v0.2.1-beta.1",
    "v0.2.1",
  ]) {
    assert.throws(
      () => validateManualAlphaVersion(version),
      /manual workflow version must match/,
    );
  }
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

test("selects only the requested immutable desktop packages for manual upload", () => {
  assert.deepEqual(
    desktopPackageUploadPaths(
      "v0.2.1-alpha.1",
      "/release",
      "darwin-arm64",
    ).map((file) => path.basename(file)),
    [
      "csgclaw-desktop_v0.2.1-alpha.1_darwin_arm64.dmg",
      "csgclaw-desktop_v0.2.1-alpha.1_darwin_arm64.zip",
    ],
  );
  assert.deepEqual(
    desktopPackageUploadPaths(
      "v0.2.1-alpha.1",
      "/release",
      "linux-arm64",
    ).map((file) => path.basename(file)),
    ["csgclaw-desktop_v0.2.1-alpha.1_linux_arm64.deb"],
  );
  assert.throws(
    () => desktopPackageUploadPaths("v0.2.1-alpha.1", "/release", ""),
    /at least one desktop package target is required/,
  );
  assert.throws(
    () =>
      desktopPackageUploadPaths(
        "v0.2.1-alpha.1",
        "/release",
        "darwin-riscv64",
      ),
    /unsupported target/,
  );
});

test("builds public download URLs for manually uploaded desktop packages", () => {
  assert.deepEqual(
    desktopPackageDownloadURLs("v0.2.1-alpha.1", "darwin-arm64"),
    [
      "https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/releases/0.2.1-alpha.1/csgclaw-desktop_v0.2.1-alpha.1_darwin_arm64.dmg",
      "https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/releases/0.2.1-alpha.1/csgclaw-desktop_v0.2.1-alpha.1_darwin_arm64.zip",
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

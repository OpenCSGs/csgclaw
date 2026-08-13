import assert from "node:assert/strict";
import test from "node:test";

import {
  normalizeMacArchitecture,
  parseDeveloperIDIdentities,
  parseEnvironmentText,
  requestedMacArchitectures,
  selectDeveloperIDIdentity,
  signingBuildEnvironment,
} from "./macos-desktop-package.mjs";

test("parses the release environment format without evaluating shell syntax", () => {
  assert.deepEqual(
    parseEnvironmentText(`
# local release credentials
APPLE_ID="release@example.com"
APPLE_PASSWORD='app-specific-password'
APPLE_TEAM_ID=TEAM123456
`),
    {
      APPLE_ID: "release@example.com",
      APPLE_PASSWORD: "app-specific-password",
      APPLE_TEAM_ID: "TEAM123456",
    },
  );
  assert.throws(
    () => parseEnvironmentText("export APPLE_ID=value"),
    /invalid environment key/,
  );
});

test("extracts and selects the Developer ID identity for the configured team", () => {
  const output = `
  1) ABCDEF "Developer ID Application: OpenCSG (TEAM123456)"
  2) FEDCBA "Developer ID Application: Other Company (OTHER12345)"
     2 valid identities found
`;
  assert.deepEqual(parseDeveloperIDIdentities(output), [
    "Developer ID Application: OpenCSG (TEAM123456)",
    "Developer ID Application: Other Company (OTHER12345)",
  ]);
  assert.equal(
    selectDeveloperIDIdentity(output, "TEAM123456"),
    "Developer ID Application: OpenCSG (TEAM123456)",
  );
  assert.throws(
    () =>
      selectDeveloperIDIdentity(
        output,
        "TEAM123456",
        "Developer ID Application: Other Company (OTHER12345)",
      ),
    /does not belong/,
  );
});

test("rejects missing or ambiguous Developer ID identities", () => {
  assert.throws(
    () => selectDeveloperIDIdentity("0 valid identities found", "TEAM123456"),
    /found 0/,
  );
  const output = `
  1) ABCDEF "Developer ID Application: OpenCSG One (TEAM123456)"
  2) FEDCBA "Developer ID Application: OpenCSG Two (TEAM123456)"
`;
  assert.throws(
    () => selectDeveloperIDIdentity(output, "TEAM123456"),
    /found 2/,
  );
});

test("normalizes Go and Electron macOS architecture names", () => {
  assert.deepEqual(normalizeMacArchitecture("arm64"), {
    goarch: "arm64",
    electronArch: "arm64",
  });
  assert.deepEqual(normalizeMacArchitecture("amd64"), {
    goarch: "amd64",
    electronArch: "x64",
  });
  assert.deepEqual(normalizeMacArchitecture("x64"), {
    goarch: "amd64",
    electronArch: "x64",
  });
  assert.throws(() => normalizeMacArchitecture("universal"), /unsupported/);
});

test("defaults local signed releases to both macOS architectures", () => {
  assert.deepEqual(requestedMacArchitectures(), [
    { goarch: "arm64", electronArch: "arm64" },
    { goarch: "amd64", electronArch: "x64" },
  ]);
  assert.deepEqual(
    requestedMacArchitectures("darwin-amd64,arm64,x64"),
    [
      { goarch: "amd64", electronArch: "x64" },
      { goarch: "arm64", electronArch: "arm64" },
    ],
  );
});

test("does not expose certificate and OSS secrets to build subprocesses", () => {
  const environment = signingBuildEnvironment(
    {
      APPLE_PASSWORD: "needed-by-forge",
      CSGCLAW_MACOS_CERTIFICATE_P12_BASE64: "certificate",
      CSGCLAW_MACOS_CERTIFICATE_PASSWORD: "certificate-password",
      OSS_ACCESS_KEY_SECRET: "oss-secret",
    },
    "Developer ID Application: OpenCSG (TEAM123456)",
  );
  assert.equal(environment.APPLE_PASSWORD, "needed-by-forge");
  assert.equal(
    environment.CSGCLAW_MACOS_SIGN_IDENTITY,
    "Developer ID Application: OpenCSG (TEAM123456)",
  );
  assert.equal(environment.CSGCLAW_MACOS_SKIP_SIGN, "0");
  assert.equal(environment.CSGCLAW_MACOS_CERTIFICATE_P12_BASE64, undefined);
  assert.equal(environment.CSGCLAW_MACOS_CERTIFICATE_PASSWORD, undefined);
  assert.equal(environment.OSS_ACCESS_KEY_SECRET, undefined);
});

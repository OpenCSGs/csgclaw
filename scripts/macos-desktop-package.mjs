#!/usr/bin/env node

import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { spawn, spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";

import { collectDesktopReleaseAssets } from "./collect-desktop-release-assets.mjs";
import {
  inferReleaseChannel,
  normalizeReleaseVersion,
  releaseTag,
} from "./desktop-release-artifacts.mjs";

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(scriptDirectory, "..");
const defaultEnvironmentFile = path.join(repositoryRoot, ".desktop-release-oss.env");
const defaultLocalOutputRoot = path.join(
  repositoryRoot,
  "desktop",
  "out",
  "local",
  "releases",
);
const defaultUpdateChannelsURL =
  "https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/channels";
const appBundleName = "CSGClaw.app";
const dmgBundleIdentifier = "com.opencsg.csgclaw.desktop.dmg";

export function parseEnvironmentText(contents, source = "environment file") {
  const values = {};
  for (const rawLine of contents.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) {
      continue;
    }
    const separator = line.indexOf("=");
    if (separator < 1) {
      throw new Error(`invalid environment line in ${source}: ${rawLine}`);
    }
    const key = line.slice(0, separator).trim();
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) {
      throw new Error(`invalid environment key in ${source}: ${key}`);
    }
    let value = line.slice(separator + 1).trim();
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1);
    }
    values[key] = value;
  }
  return values;
}

export function parseDeveloperIDIdentities(output) {
  return String(output)
    .split(/\r?\n/)
    .map((line) => /"(Developer ID Application:[^"]+)"/.exec(line)?.[1] || "")
    .filter(Boolean);
}

export function selectDeveloperIDIdentity(output, teamID, requestedIdentity = "") {
  const normalizedTeamID = requiredValue({ APPLE_TEAM_ID: teamID }, "APPLE_TEAM_ID");
  const identities = [...new Set(parseDeveloperIDIdentities(output))];
  const requested = String(requestedIdentity).trim();
  if (requested) {
    if (!identities.includes(requested)) {
      throw new Error(
        `CSGCLAW_MACOS_SIGN_IDENTITY is not a valid Developer ID identity in the active keychain: ${requested}`,
      );
    }
    if (!requested.endsWith(`(${normalizedTeamID})`)) {
      throw new Error(
        `the requested Developer ID identity does not belong to APPLE_TEAM_ID ${normalizedTeamID}`,
      );
    }
    return requested;
  }

  const matches = identities.filter((identity) =>
    identity.endsWith(`(${normalizedTeamID})`),
  );
  if (matches.length !== 1) {
    throw new Error(
      `expected exactly one valid Developer ID Application identity for APPLE_TEAM_ID ${normalizedTeamID}; found ${matches.length}`,
    );
  }
  return matches[0];
}

export function normalizeMacArchitecture(value) {
  switch (String(value).trim()) {
    case "darwin-arm64":
    case "arm64":
      return { goarch: "arm64", electronArch: "arm64" };
    case "darwin-amd64":
    case "amd64":
    case "x64":
      return { goarch: "amd64", electronArch: "x64" };
    default:
      throw new Error(`unsupported macOS architecture: ${value}`);
  }
}

export function requestedMacArchitectures(rawTargets = "arm64,amd64") {
  const values = String(rawTargets)
    .split(",")
    .map((value) => value.trim())
    .filter(Boolean);
  if (values.length === 0) {
    throw new Error("at least one macOS architecture is required");
  }
  const architectures = values.map(normalizeMacArchitecture);
  return architectures.filter(
    (architecture, index) =>
      architectures.findIndex(
        (candidate) => candidate.goarch === architecture.goarch,
      ) === index,
  );
}

export function signingBuildEnvironment(environment, identity) {
  const childEnvironment = {
    ...environment,
    CSGCLAW_MACOS_SIGN_IDENTITY: identity,
    CSGCLAW_MACOS_SKIP_SIGN: "0",
  };
  for (const name of [
    "CSGCLAW_MACOS_CERTIFICATE_P12_FILE",
    "CSGCLAW_MACOS_CERTIFICATE_P12_BASE64",
    "CSGCLAW_MACOS_CERTIFICATE_PASSWORD",
    "OSS_ACCESS_KEY_ID",
    "OSS_ACCESS_KEY_SECRET",
  ]) {
    delete childEnvironment[name];
  }
  return childEnvironment;
}

function parseOptions(args) {
  const options = {};
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === "--help") {
      options.help = true;
      continue;
    }
    if (argument === "--force") {
      options.force = true;
      continue;
    }
    if (!argument.startsWith("--")) {
      throw new Error(`unexpected argument: ${argument}`);
    }
    const value = args[index + 1];
    if (!value || value.startsWith("--")) {
      throw new Error(`missing value for ${argument}`);
    }
    options[argument.slice(2)] = value;
    index += 1;
  }
  return options;
}

function loadEnvironment(environmentFile, baseEnvironment) {
  let fileValues = {};
  const explicitFile = String(environmentFile || "").trim();
  const selectedFile = explicitFile || defaultEnvironmentFile;
  if (fs.existsSync(selectedFile)) {
    const mode = fs.statSync(selectedFile).mode & 0o777;
    if ((mode & 0o077) !== 0) {
      console.warn(
        `warning: ${selectedFile} is readable by other users; run chmod 600 before storing release credentials`,
      );
    }
    fileValues = parseEnvironmentText(fs.readFileSync(selectedFile, "utf8"), selectedFile);
    console.log(`Loaded release credentials from ${selectedFile}.`);
  } else if (explicitFile) {
    throw new Error(`release environment file does not exist: ${selectedFile}`);
  }
  return { ...fileValues, ...baseEnvironment };
}

function requiredValue(environment, name) {
  const value = String(environment[name] || "").trim();
  if (!value) {
    throw new Error(`required release credential is empty: ${name}`);
  }
  return value;
}

function run(command, args, { environment, capture = false } = {}) {
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    env: environment || process.env,
    encoding: capture ? "utf8" : undefined,
    stdio: capture ? ["ignore", "pipe", "pipe"] : "inherit",
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    const detail = capture ? String(result.stderr || result.stdout || "").trim() : "";
    throw new Error(
      `${command} failed with exit code ${result.status}${detail ? `: ${detail}` : ""}`,
    );
  }
  return capture ? String(result.stdout || "") : "";
}

function runAsync(command, args, { environment } = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: repositoryRoot,
      env: environment || process.env,
      stdio: "inherit",
    });
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`${command} failed with exit code ${code}`));
    });
  });
}

function parseKeychainList(output) {
  return String(output)
    .split(/\r?\n/)
    .map((line) => line.trim().replace(/^"|"$/g, ""))
    .filter(Boolean);
}

function decodeCertificate(contents) {
  const normalized = String(contents).replace(/\s+/g, "");
  if (!normalized || !/^[A-Za-z0-9+/]+={0,2}$/.test(normalized)) {
    throw new Error("CSGCLAW_MACOS_CERTIFICATE_P12_BASE64 is not valid base64");
  }
  const certificate = Buffer.from(normalized, "base64");
  if (certificate.length === 0) {
    throw new Error("CSGCLAW_MACOS_CERTIFICATE_P12_BASE64 decoded to an empty file");
  }
  return certificate;
}

function prepareSigningIdentity(environment) {
  const teamID = requiredValue(environment, "APPLE_TEAM_ID");
  const requestedIdentity = String(environment.CSGCLAW_MACOS_SIGN_IDENTITY || "").trim();
  const certificateFile = String(
    environment.CSGCLAW_MACOS_CERTIFICATE_P12_FILE || "",
  ).trim();
  const certificateBase64 = String(
    environment.CSGCLAW_MACOS_CERTIFICATE_P12_BASE64 || "",
  ).trim();

  if (certificateFile && certificateBase64) {
    throw new Error(
      "configure only one of CSGCLAW_MACOS_CERTIFICATE_P12_FILE or CSGCLAW_MACOS_CERTIFICATE_P12_BASE64",
    );
  }
  if (!certificateFile && !certificateBase64) {
    const output = run("security", ["find-identity", "-v", "-p", "codesigning"], {
      environment,
      capture: true,
    });
    return {
      identity: selectDeveloperIDIdentity(output, teamID, requestedIdentity),
      cleanup() {},
    };
  }

  const certificatePassword = requiredValue(
    environment,
    "CSGCLAW_MACOS_CERTIFICATE_PASSWORD",
  );
  const temporaryDirectory = fs.mkdtempSync(
    path.join(os.tmpdir(), "csgclaw-macos-signing-"),
  );
  const importedCertificate = path.join(temporaryDirectory, "signing.p12");
  const keychainPath = path.join(temporaryDirectory, "signing.keychain-db");
  const keychainPassword = crypto.randomBytes(32).toString("hex");
  const originalKeychains = parseKeychainList(
    run("security", ["list-keychains", "-d", "user"], {
      environment,
      capture: true,
    }),
  );

  try {
    if (certificateFile) {
      const source = path.resolve(certificateFile);
      if (!fs.statSync(source).isFile()) {
        throw new Error(`macOS signing certificate is not a file: ${source}`);
      }
      fs.copyFileSync(source, importedCertificate);
    } else {
      fs.writeFileSync(importedCertificate, decodeCertificate(certificateBase64));
    }
    fs.chmodSync(importedCertificate, 0o600);

    run("security", ["create-keychain", "-p", keychainPassword, keychainPath], {
      environment,
      capture: true,
    });
    run("security", ["set-keychain-settings", "-lut", "21600", keychainPath], {
      environment,
      capture: true,
    });
    run("security", ["unlock-keychain", "-p", keychainPassword, keychainPath], {
      environment,
      capture: true,
    });
    run(
      "security",
      [
        "import",
        importedCertificate,
        "-k",
        keychainPath,
        "-P",
        certificatePassword,
        "-T",
        "/usr/bin/codesign",
        "-T",
        "/usr/bin/security",
      ],
      { environment, capture: true },
    );
    run(
      "security",
      [
        "set-key-partition-list",
        "-S",
        "apple-tool:,apple:",
        "-s",
        "-k",
        keychainPassword,
        keychainPath,
      ],
      { environment, capture: true },
    );
    run(
      "security",
      ["list-keychains", "-d", "user", "-s", keychainPath, ...originalKeychains],
      { environment, capture: true },
    );
    const output = run(
      "security",
      ["find-identity", "-v", "-p", "codesigning", keychainPath],
      { environment, capture: true },
    );
    const identity = selectDeveloperIDIdentity(output, teamID, requestedIdentity);
    return {
      identity,
      cleanup() {
        restoreAndDeleteKeychain(environment, originalKeychains, keychainPath);
        fs.rmSync(temporaryDirectory, { recursive: true, force: true });
      },
    };
  } catch (error) {
    restoreAndDeleteKeychain(environment, originalKeychains, keychainPath);
    fs.rmSync(temporaryDirectory, { recursive: true, force: true });
    throw error;
  }
}

function restoreAndDeleteKeychain(environment, originalKeychains, keychainPath) {
  if (originalKeychains.length > 0) {
    spawnSync(
      "security",
      ["list-keychains", "-d", "user", "-s", ...originalKeychains],
      { cwd: repositoryRoot, env: environment, stdio: "ignore" },
    );
  }
  spawnSync("security", ["delete-keychain", keychainPath], {
    cwd: repositoryRoot,
    env: environment,
    stdio: "ignore",
  });
}

function findFiles(directory, matches) {
  if (!fs.existsSync(directory)) {
    return [];
  }
  return fs
    .readdirSync(directory, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => path.join(entry.parentPath, entry.name))
    .filter(matches);
}

function exactlyOne(values, description) {
  if (values.length !== 1) {
    throw new Error(`expected exactly one ${description}; found ${values.length}`);
  }
  return values[0];
}

function verifyApplication(appPath, environment) {
  run("codesign", ["--verify", "--deep", "--strict", "--verbose=2", appPath], {
    environment,
  });
  run("xcrun", ["stapler", "validate", appPath], { environment });
  run("spctl", ["--assess", "--type", "execute", "--verbose=4", appPath], {
    environment,
  });
}

function signDiskImage(dmgPath, identity, environment) {
  console.log(`Signing ${dmgPath}.`);
  run(
    "codesign",
    [
      "--force",
      "--timestamp",
      "--identifier",
      dmgBundleIdentifier,
      "--sign",
      identity,
      dmgPath,
    ],
    { environment },
  );
}

function submitDiskImageNotarization(dmgPath, environment) {
  console.log(`Submitting ${dmgPath} for notarization.`);
  return runAsync(
    "xcrun",
    [
      "notarytool",
      "submit",
      dmgPath,
      "--apple-id",
      requiredValue(environment, "APPLE_ID"),
      "--password",
      requiredValue(environment, "APPLE_PASSWORD"),
      "--team-id",
      requiredValue(environment, "APPLE_TEAM_ID"),
      "--wait",
    ],
    { environment },
  );
}

function stapleAndVerifyDiskImage(dmgPath, environment) {
  run("xcrun", ["stapler", "staple", dmgPath], { environment });
  run("xcrun", ["stapler", "validate", dmgPath], { environment });
  run(
    "spctl",
    [
      "--assess",
      "--type",
      "open",
      "--context",
      "context:primary-signature",
      "--verbose=4",
      dmgPath,
    ],
    { environment },
  );
}

function verifyUpdateArchive(zipPath, environment) {
  const temporaryDirectory = fs.mkdtempSync(
    path.join(os.tmpdir(), "csgclaw-macos-update-"),
  );
  try {
    run("ditto", ["-x", "-k", zipPath, temporaryDirectory], { environment });
    const appPath = exactlyOne(
      fs
        .readdirSync(temporaryDirectory, { recursive: true, withFileTypes: true })
        .filter((entry) => entry.isDirectory() && entry.name === appBundleName)
        .map((entry) => path.join(entry.parentPath, entry.name)),
      `signed ${appBundleName} in the update ZIP`,
    );
    verifyApplication(appPath, environment);
  } finally {
    fs.rmSync(temporaryDirectory, { recursive: true, force: true });
  }
}

function prepareLocalReleaseDirectory(releaseDirectory, force) {
  if (!fs.existsSync(releaseDirectory)) {
    fs.mkdirSync(releaseDirectory, { recursive: true });
    return;
  }
  if (fs.readdirSync(releaseDirectory).length === 0) {
    return;
  }
  if (!force) {
    throw new Error(
      `local release directory already contains files; set DESKTOP_MACOS_FORCE=1 to rebuild: ${releaseDirectory}`,
    );
  }
  const relativePath = path.relative(defaultLocalOutputRoot, releaseDirectory);
  if (
    !relativePath ||
    relativePath.startsWith("..") ||
    path.isAbsolute(relativePath)
  ) {
    throw new Error(
      `refusing to force-remove a release directory outside ${defaultLocalOutputRoot}: ${releaseDirectory}`,
    );
  }
  fs.rmSync(releaseDirectory, { recursive: true, force: true });
  fs.mkdirSync(releaseDirectory, { recursive: true });
}

async function packageArchitecture({
  architecture,
  version,
  releaseDirectory,
  signingIdentity,
  environment,
}) {
  console.log(
    `Building signed macOS desktop package ${version} (${architecture.goarch}).`,
  );
  run(
    "make",
    [
      "desktop-package",
      "TARGET_OS=darwin",
      `TARGET_ARCH=${architecture.goarch}`,
      `VERSION=${version}`,
    ],
    { environment },
  );

  const appPath = path.join(
    repositoryRoot,
    "desktop",
    "out",
    `CSGClaw-darwin-${architecture.electronArch}`,
    appBundleName,
  );
  if (!fs.existsSync(appPath)) {
    throw new Error(`packaged application does not exist: ${appPath}`);
  }

  const makeDirectory = path.join(repositoryRoot, "desktop", "out", "make");
  const dmgPath = exactlyOne(
    findFiles(makeDirectory, (filePath) => filePath.endsWith(".dmg")),
    "macOS DMG",
  );
  const zipPath = exactlyOne(
    findFiles(makeDirectory, (filePath) => filePath.endsWith(".zip")),
    "macOS update ZIP",
  );
  signDiskImage(dmgPath, signingIdentity, environment);
  const notarizingDiskImage = submitDiskImageNotarization(dmgPath, environment);
  console.log("Verifying the app and update ZIP while Apple notarizes the DMG.");
  verifyApplication(appPath, environment);
  verifyUpdateArchive(zipPath, environment);
  await notarizingDiskImage;
  stapleAndVerifyDiskImage(dmgPath, environment);

  const staged = collectDesktopReleaseAssets({
    version,
    goos: "darwin",
    goarch: architecture.goarch,
    makeDirectory,
    outputDirectory: releaseDirectory,
  });
  for (const filePath of staged) {
    console.log(`staged ${filePath}`);
  }
  console.log(
    `Signed ${architecture.goarch} package passed app, ZIP, and DMG verification.`,
  );
}

async function packageSignedDesktop(options) {
  if (process.platform !== "darwin") {
    throw new Error("signed macOS desktop packages must be built on macOS");
  }
  if (options.arch && options.targets) {
    throw new Error("configure only one of --arch or --targets");
  }
  const normalizedVersion = normalizeReleaseVersion(
    options.version || process.env.VERSION,
  );
  const version = releaseTag(normalizedVersion);
  const architectures = requestedMacArchitectures(
    options.targets ||
      options.arch ||
      process.env.DESKTOP_MACOS_TARGETS ||
      "arm64,amd64",
  );
  const releaseDirectory = path.resolve(
    options["release-directory"] ||
      path.join(defaultLocalOutputRoot, normalizedVersion),
  );
  if (
    fs.existsSync(releaseDirectory) &&
    fs.readdirSync(releaseDirectory).length > 0 &&
    !options.force
  ) {
    throw new Error(
      `local release directory already contains files; set DESKTOP_MACOS_FORCE=1 to rebuild: ${releaseDirectory}`,
    );
  }

  const environment = loadEnvironment(options["env-file"], process.env);
  requiredValue(environment, "APPLE_ID");
  requiredValue(environment, "APPLE_PASSWORD");
  requiredValue(environment, "APPLE_TEAM_ID");
  if (environment.CSGCLAW_MACOS_SKIP_SIGN === "1") {
    throw new Error("CSGCLAW_MACOS_SKIP_SIGN=1 is incompatible with signed packaging");
  }

  const signing = prepareSigningIdentity(environment);
  const signingEnvironment = signingBuildEnvironment(environment, signing.identity);
  const updateChannelsURL = String(
    signingEnvironment.CSGCLAW_DESKTOP_UPDATE_CHANNELS_URL ||
      defaultUpdateChannelsURL,
  ).replace(/\/+$/, "");
  signingEnvironment.CSGCLAW_DESKTOP_UPDATE_BASE_URL =
    signingEnvironment.CSGCLAW_DESKTOP_UPDATE_BASE_URL ||
    `${updateChannelsURL}/${inferReleaseChannel(normalizedVersion)}/updates`;
  try {
    console.log(`Using ${signing.identity}.`);
    prepareLocalReleaseDirectory(releaseDirectory, options.force);
    for (const architecture of architectures) {
      await packageArchitecture({
        architecture,
        version,
        releaseDirectory,
        signingIdentity: signing.identity,
        environment: signingEnvironment,
      });
    }
    console.log(`Local signed release ready: ${releaseDirectory}`);
    console.log("No OSS upload was performed.");
  } finally {
    signing.cleanup();
  }
}

function printHelp() {
  console.log(`Usage:
  node scripts/macos-desktop-package.mjs --version <semver> [--targets <arm64,amd64>] [--release-directory <path>] [--env-file <path>] [--force]

Credentials are read from the process environment and optionally from
.desktop-release-oss.env. Process environment values take precedence.

This command only creates local release files. It never uploads to OSS.
`);
}

function main(args) {
  const options = parseOptions(args);
  if (options.help) {
    printHelp();
    return Promise.resolve();
  }
  return packageSignedDesktop(options);
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}

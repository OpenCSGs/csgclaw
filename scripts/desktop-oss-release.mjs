#!/usr/bin/env node

import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";

import { collectDesktopReleaseAssets } from "./collect-desktop-release-assets.mjs";
import {
  desktopDownloadArtifacts,
  desktopReleaseArtifactNames,
  inferReleaseChannel,
  normalizeReleaseVersion,
  releaseTag,
  validateReleaseChannel,
} from "./desktop-release-artifacts.mjs";

export {
  inferReleaseChannel,
  normalizeReleaseVersion,
  releaseTag,
  validateReleaseChannel,
} from "./desktop-release-artifacts.mjs";

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const repositoryRoot = path.resolve(scriptDirectory, "..");
const defaultOutputRoot = path.join(repositoryRoot, "desktop", "out", "oss");
const defaultEnvironmentFile = path.join(repositoryRoot, ".desktop-release-oss.env");
const defaultBucket = "opencsg-public-resource";
const defaultPrefix = "csgclaw-desktop";
const defaultPublicBaseURL =
  "https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop";

const targetDefinitions = {
  "darwin-arm64": { goos: "darwin", goarch: "arm64", host: "darwin" },
  "darwin-amd64": { goos: "darwin", goarch: "amd64", host: "darwin" },
  "windows-amd64": { goos: "windows", goarch: "amd64", host: "win32" },
};

export function desktopUploadPaths(version, releaseDirectory) {
  return desktopDownloadArtifacts(version).map(({ fileName }) =>
    path.join(releaseDirectory, fileName));
}

export function generateDownloadsManifest({
  version,
  channel,
  releaseDirectory,
  manifestPath,
  publicBaseURL = defaultPublicBaseURL,
  publishedAt = new Date().toISOString(),
  allowPartial = false,
}) {
  validateReleaseChannel(version, channel);
  const artifacts = [];
  const missing = [];

  for (const definition of desktopDownloadArtifacts(version)) {
    const { fileName } = definition;
    const filePath = path.join(releaseDirectory, fileName);
    if (!fs.existsSync(filePath)) {
      missing.push(fileName);
      continue;
    }
    const stat = fs.statSync(filePath);
    if (!stat.isFile() || stat.size === 0) {
      throw new Error(`release artifact is empty or not a file: ${filePath}`);
    }
    artifacts.push({
      platform: definition.platform,
      arch: definition.arch,
      url: `${publicBaseURL.replace(/\/+$/, "")}/releases/${encodeURIComponent(version)}/${fileName}`,
      size_bytes: stat.size,
      sha256: sha256File(filePath),
    });
  }

  if (!allowPartial && missing.length > 0) {
    throw new Error(`cannot generate complete downloads.json; missing: ${missing.join(", ")}`);
  }
  if (artifacts.length === 0) {
    throw new Error(`no desktop installers found in ${releaseDirectory}`);
  }

  const previous = readExistingManifest(manifestPath, channel);
  const versions = {
    [version]: {
      version,
      published_at: publishedAt,
      artifacts,
    },
  };
  for (const [existingVersion, value] of Object.entries(previous?.versions || {})) {
    if (existingVersion !== version) {
      versions[existingVersion] = value;
    }
  }

  const manifest = {
    schema_version: 1,
    channel,
    latest: version,
    versions,
  };
  fs.mkdirSync(path.dirname(manifestPath), { recursive: true });
  fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  return manifest;
}

function sha256File(filePath) {
  const hash = crypto.createHash("sha256");
  hash.update(fs.readFileSync(filePath));
  return hash.digest("hex");
}

function readExistingManifest(manifestPath, channel) {
  if (!fs.existsSync(manifestPath)) {
    return null;
  }
  const parsed = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  if (parsed.channel !== channel || parsed.schema_version !== 1 || typeof parsed.versions !== "object") {
    throw new Error(`existing manifest has an incompatible schema: ${manifestPath}`);
  }
  return parsed;
}

function parseOptions(args) {
  const options = {};
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === "--force" || argument === "--allow-partial") {
      options[argument.slice(2)] = true;
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

function releaseContext(options) {
  const version = normalizeReleaseVersion(options.version || process.env.VERSION);
  const channel = String(
    options.channel || process.env.DESKTOP_RELEASE_CHANNEL || inferReleaseChannel(version),
  ).trim();
  validateReleaseChannel(version, channel);
  const outputRoot = path.resolve(options["output-root"] || defaultOutputRoot);
  return {
    version,
    channel,
    outputRoot,
    releaseDirectory: options["release-directory"]
      ? path.resolve(options["release-directory"])
      : path.join(outputRoot, "releases", version),
    manifestPath: path.join(outputRoot, "channels", channel, "downloads.json"),
  };
}

function defaultTargets() {
  if (process.platform === "darwin") {
    return ["darwin-arm64", "darwin-amd64"];
  }
  if (process.platform === "win32") {
    return ["windows-amd64"];
  }
  throw new Error(`desktop installer builds are not supported on ${process.platform}/${process.arch}`);
}

function requestedTargets(rawTargets) {
  const targets = rawTargets
    ? String(rawTargets).split(",").map((value) => value.trim()).filter(Boolean)
    : defaultTargets();
  for (const target of targets) {
    const definition = targetDefinitions[target];
    if (!definition) {
      throw new Error(`unsupported target: ${target}`);
    }
    if (definition.host !== process.platform) {
      const hostName = {
        darwin: "macOS",
        linux: "Linux",
        win32: "Windows",
      }[definition.host];
      throw new Error(
        `${target} must be built on ${hostName}; current host is ${process.platform}`,
      );
    }
  }
  return targets;
}

function buildDesktopInstallers(context, options) {
  const targets = requestedTargets(options.targets);
  fs.mkdirSync(context.releaseDirectory, { recursive: true });

  for (const target of targets) {
    const definition = targetDefinitions[target];
    const versionTag = releaseTag(context.version);
    for (const destination of desktopReleaseArtifactNames({
      version: versionTag,
      goos: definition.goos,
      goarch: definition.goarch,
    })) {
      const destinationPath = path.join(context.releaseDirectory, destination);
      if (options.force) {
        fs.rmSync(destinationPath, { force: true });
      } else if (fs.existsSync(destinationPath)) {
        throw new Error(`staged artifact already exists; use --force to rebuild: ${destinationPath}`);
      }
    }

    const makeDirectory = path.join(repositoryRoot, "desktop", "out", "make");
    fs.rmSync(makeDirectory, { recursive: true, force: true });
    const environment = {
      ...process.env,
      VERSION: versionTag,
      TARGET_OS: definition.goos,
      TARGET_ARCH: definition.goarch,
    };
    if (
      process.platform === "darwin" &&
      process.arch === "arm64" &&
      definition.goos === "darwin" &&
      definition.goarch === "amd64" &&
      !environment.CSGCLAW_MACOS_SIGN_IDENTITY &&
      !(environment.APPLE_ID && environment.APPLE_PASSWORD && environment.APPLE_TEAM_ID)
    ) {
      environment.CSGCLAW_MACOS_SKIP_SIGN = "1";
      console.warn("building the Intel package without a temporary signature; sign and notarize it before public release");
    }

    if (process.platform !== "win32") {
      runCommand(
        "make",
        [
          "desktop-package",
          `TARGET_OS=${definition.goos}`,
          `TARGET_ARCH=${definition.goarch}`,
          `VERSION=${versionTag}`,
        ],
        environment,
      );
    } else {
      runCommand(
        "cmd.exe",
        ["/d", "/s", "/c", path.join(repositoryRoot, "scripts", "build.cmd"), "desktop-package"],
        environment,
      );
    }

    const staged = collectDesktopReleaseAssets({
      version: versionTag,
      goos: definition.goos,
      goarch: definition.goarch,
      makeDirectory,
      outputDirectory: context.releaseDirectory,
    });
    for (const filePath of staged) {
      console.log(`staged ${filePath}`);
    }
  }
}

function runCommand(command, args, environment = process.env) {
  console.log(`> ${command} ${args.join(" ")}`);
  const result = spawnSync(command, args, {
    cwd: repositoryRoot,
    env: environment,
    stdio: "inherit",
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(`${command} failed with exit code ${result.status}`);
  }
}

function parseEnvironmentFile(filePath) {
  if (!fs.existsSync(filePath)) {
    return {};
  }
  const values = {};
  for (const rawLine of fs.readFileSync(filePath, "utf8").split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#")) {
      continue;
    }
    const separator = line.indexOf("=");
    if (separator < 1) {
      throw new Error(`invalid environment line in ${filePath}: ${rawLine}`);
    }
    const key = line.slice(0, separator).trim();
    let value = line.slice(separator + 1).trim();
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }
    values[key] = value;
  }
  return values;
}

async function uploadDesktopRelease(context, options) {
  const environmentFile = path.resolve(options["env-file"] || defaultEnvironmentFile);
  const fileEnvironment = parseEnvironmentFile(environmentFile);
  const setting = (name, fallback = "") => process.env[name] || fileEnvironment[name] || fallback;
  const accessKeyID = setting("OSS_ACCESS_KEY_ID");
  const accessKeySecret = setting("OSS_ACCESS_KEY_SECRET");
  if (!accessKeyID || !accessKeySecret) {
    throw new Error(
      `OSS credentials are empty; copy .desktop-release-oss.env.example to ${path.basename(environmentFile)} and fill it locally`,
    );
  }

  const bucket = setting("OSS_BUCKET", defaultBucket);
  const prefix = setting("OSS_PREFIX", defaultPrefix).replace(/^\/+|\/+$/g, "");
  const region = setting("OSS_REGION", "cn-beijing");
  const endpoint = setting("OSS_ENDPOINT", "https://oss-cn-beijing.aliyuncs.com");
  const publicBaseURL = setting("OSS_PUBLIC_BASE_URL", defaultPublicBaseURL).replace(/\/+$/, "");
  const ossEnvironment = {
    ...process.env,
    OSS_ACCESS_KEY_ID: accessKeyID,
    OSS_ACCESS_KEY_SECRET: accessKeySecret,
    OSS_REGION: region,
    OSS_ENDPOINT: endpoint,
  };

  runCommand("ossutil", ["version"], ossEnvironment);
  const manifestURL = `${publicBaseURL}/channels/${context.channel}/downloads.json`;
  const currentResponse = await fetch(`${manifestURL}?current=${Date.now()}`, { cache: "no-store" });
  if (currentResponse.ok) {
    const currentManifest = await currentResponse.json();
    if (
      currentManifest.schema_version !== 1 ||
      currentManifest.channel !== context.channel ||
      typeof currentManifest.versions !== "object"
    ) {
      throw new Error(`remote manifest has an incompatible schema: ${manifestURL}`);
    }
    fs.mkdirSync(path.dirname(context.manifestPath), { recursive: true });
    fs.writeFileSync(context.manifestPath, `${JSON.stringify(currentManifest, null, 2)}\n`);
  } else if (currentResponse.status !== 404) {
    throw new Error(`cannot read current manifest: HTTP ${currentResponse.status} ${manifestURL}`);
  }
  generateDownloadsManifest({
    ...context,
    publicBaseURL,
    allowPartial: Boolean(options["allow-partial"]),
  });

  // GitHub Release also contains macOS ZIP and Linux packages. The website
  // manifest intentionally exposes only the two DMGs and the Windows setup
  // executable, so keep the OSS object set aligned with that public contract.
  const releaseFiles = desktopUploadPaths(context.version, context.releaseDirectory);
  for (const filePath of releaseFiles) {
    const objectPath = `oss://${bucket}/${prefix}/releases/${context.version}/${path.basename(filePath)}`;
    runCommand(
      "ossutil",
      ["cp", filePath, objectPath, "-u", "--cache-control", "public,max-age=31536000,immutable"],
      ossEnvironment,
    );
  }

  const manifestObject = `oss://${bucket}/${prefix}/channels/${context.channel}/downloads.json`;
  runCommand(
    "ossutil",
    [
      "cp",
      context.manifestPath,
      manifestObject,
      "-f",
      "--content-type",
      "application/json",
      "--cache-control",
      "no-cache",
    ],
    ossEnvironment,
  );

  const response = await fetch(`${manifestURL}?verify=${Date.now()}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`uploaded manifest verification failed: HTTP ${response.status} ${manifestURL}`);
  }
  const uploaded = await response.json();
  if (uploaded.latest !== context.version) {
    throw new Error(`uploaded manifest latest is ${uploaded.latest}, expected ${context.version}`);
  }
  console.log(`verified ${manifestURL}`);
}

function printHelp() {
  console.log(`Usage:
  node scripts/desktop-oss-release.mjs build --version <semver> [--channel <beta|release>] [--targets <list>] [--release-directory <path>] [--force]
  node scripts/desktop-oss-release.mjs manifest --version <semver> [--channel <beta|release>] [--release-directory <path>] [--allow-partial]
  node scripts/desktop-oss-release.mjs upload --version <semver> [--channel <beta|release>] [--release-directory <path>] [--env-file <path>]

Targets:
  darwin-arm64,darwin-amd64  Build on macOS
  windows-amd64              Build on Windows

The channel defaults to beta for prerelease versions and release for stable versions.
`);
}

async function main(args) {
  const [command, ...optionArgs] = args;
  if (!command || command === "help" || command === "--help") {
    printHelp();
    return;
  }
  const options = parseOptions(optionArgs);
  const context = releaseContext(options);

  switch (command) {
    case "build":
      buildDesktopInstallers(context, options);
      break;
    case "manifest": {
      const manifest = generateDownloadsManifest({
        ...context,
        publicBaseURL: options["public-base-url"] || defaultPublicBaseURL,
        allowPartial: Boolean(options["allow-partial"]),
      });
      console.log(`wrote ${context.manifestPath} with ${manifest.versions[context.version].artifacts.length} installers`);
      break;
    }
    case "upload":
      await uploadDesktopRelease(context, options);
      break;
    default:
      throw new Error(`unknown command: ${command}`);
  }
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}

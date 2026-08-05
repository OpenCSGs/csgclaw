import process from "node:process";
import { pathToFileURL } from "node:url";

const semanticVersionPattern =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;

const targetArtifacts = {
  "linux-amd64": ["deb"],
  "linux-arm64": ["deb"],
  "darwin-arm64": ["dmg", "zip"],
  "darwin-amd64": ["dmg", "zip"],
  "windows-amd64": ["exe"],
};

function parseReleaseVersion(rawVersion) {
  const requested = String(rawVersion || "").trim().replace(/^v/, "");
  const match = semanticVersionPattern.exec(requested);
  if (!match) {
    throw new Error(`invalid release version: ${rawVersion || "<empty>"}`);
  }
  const prereleaseIdentifiers = match[4] ? match[4].split(".") : [];
  if (
    prereleaseIdentifiers.some(
      (identifier) => /^\d+$/.test(identifier) && identifier.length > 1 && identifier.startsWith("0"),
    )
  ) {
    throw new Error(`invalid release version: ${rawVersion || "<empty>"}`);
  }
  const prerelease = match[4] ? `-${match[4]}` : "";
  return {
    normalized: `${match[1]}.${match[2]}.${match[3]}${prerelease}`,
    core: [BigInt(match[1]), BigInt(match[2]), BigInt(match[3])],
    prerelease: prereleaseIdentifiers,
  };
}

export function normalizeReleaseVersion(rawVersion) {
  return parseReleaseVersion(rawVersion).normalized;
}

export function compareReleaseVersions(leftVersion, rightVersion) {
  const left = parseReleaseVersion(leftVersion);
  const right = parseReleaseVersion(rightVersion);

  for (let index = 0; index < left.core.length; index += 1) {
    if (left.core[index] < right.core[index]) {
      return -1;
    }
    if (left.core[index] > right.core[index]) {
      return 1;
    }
  }

  if (left.prerelease.length === 0 || right.prerelease.length === 0) {
    if (left.prerelease.length === right.prerelease.length) {
      return 0;
    }
    return left.prerelease.length === 0 ? 1 : -1;
  }

  const identifierCount = Math.min(left.prerelease.length, right.prerelease.length);
  for (let index = 0; index < identifierCount; index += 1) {
    const leftIdentifier = left.prerelease[index];
    const rightIdentifier = right.prerelease[index];
    if (leftIdentifier === rightIdentifier) {
      continue;
    }
    const leftNumeric = /^\d+$/.test(leftIdentifier);
    const rightNumeric = /^\d+$/.test(rightIdentifier);
    if (leftNumeric && rightNumeric) {
      return BigInt(leftIdentifier) < BigInt(rightIdentifier) ? -1 : 1;
    }
    if (leftNumeric !== rightNumeric) {
      return leftNumeric ? -1 : 1;
    }
    return leftIdentifier < rightIdentifier ? -1 : 1;
  }

  if (left.prerelease.length === right.prerelease.length) {
    return 0;
  }
  return left.prerelease.length < right.prerelease.length ? -1 : 1;
}

export function releaseTag(version) {
  return `v${normalizeReleaseVersion(version)}`;
}

export function inferReleaseChannel(version) {
  return normalizeReleaseVersion(version).includes("-") ? "beta" : "release";
}

export function validateReleaseChannel(version, channel) {
  if (channel !== "beta" && channel !== "release") {
    throw new Error(`channel must be beta or release, got: ${channel}`);
  }
  const prerelease = normalizeReleaseVersion(version).includes("-");
  if (channel === "beta" && !prerelease) {
    throw new Error(`beta channel requires a prerelease version such as ${version}-beta.1`);
  }
  if (channel === "release" && prerelease) {
    throw new Error("release channel cannot publish a prerelease version");
  }
}

export function desktopReleaseArtifactNames({ version, goos, goarch }) {
  const extensions = targetArtifacts[`${goos}-${goarch}`];
  if (!extensions) {
    throw new Error(`unsupported desktop release target: ${goos}/${goarch}`);
  }
  const prefix = `csgclaw-desktop_${releaseTag(version)}_${goos}_${goarch}`;
  return extensions.map((extension) => `${prefix}.${extension}`);
}

export function desktopDownloadArtifacts(version) {
  return [
    {
      fileName: desktopReleaseArtifactNames({ version, goos: "darwin", goarch: "arm64" })[0],
      platform: "macos",
      arch: "arm64",
    },
    {
      fileName: desktopReleaseArtifactNames({ version, goos: "darwin", goarch: "amd64" })[0],
      platform: "macos",
      arch: "x86_64",
    },
    {
      fileName: desktopReleaseArtifactNames({ version, goos: "windows", goarch: "amd64" })[0],
      platform: "windows",
      arch: "x86_64",
    },
  ];
}

function main(args) {
  const [command, version] = args;
  if (command !== "channel" || !version || args.length !== 2) {
    throw new Error("usage: desktop-release-artifacts.mjs channel <version>");
  }
  process.stdout.write(`${inferReleaseChannel(version)}\n`);
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  }
}

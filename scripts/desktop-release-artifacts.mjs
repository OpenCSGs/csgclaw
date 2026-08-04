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

export function normalizeReleaseVersion(rawVersion) {
  const requested = String(rawVersion || "").trim().replace(/^v/, "");
  const match = semanticVersionPattern.exec(requested);
  if (!match) {
    throw new Error(`invalid release version: ${rawVersion || "<empty>"}`);
  }
  const prerelease = match[4] ? `-${match[4]}` : "";
  return `${match[1]}.${match[2]}.${match[3]}${prerelease}`;
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

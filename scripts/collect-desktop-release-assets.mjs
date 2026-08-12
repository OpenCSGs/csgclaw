#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

import {
  desktopReleaseArtifactNames,
  normalizeReleaseVersion,
} from "./desktop-release-artifacts.mjs";

export function collectDesktopReleaseAssets({ version, goos, goarch, makeDirectory, outputDirectory }) {
  const files = listFiles(makeDirectory);
  const names = desktopReleaseArtifactNames({ version, goos, goarch });
  const assets = releaseAssetsFor(goos, goarch, files, names, normalizeReleaseVersion(version));
  const updateAssets = updateAssetsFor(goos, goarch, files, normalizeReleaseVersion(version));

  fs.mkdirSync(outputDirectory, { recursive: true });
  for (const asset of assets) {
    const destination = path.join(outputDirectory, asset.name);
    fs.copyFileSync(asset.source, destination, fs.constants.COPYFILE_EXCL);
  }
  for (const asset of updateAssets) {
    const destination = path.join(outputDirectory, "updates", asset.platform, asset.arch, asset.name);
    fs.mkdirSync(path.dirname(destination), { recursive: true });
    fs.copyFileSync(asset.source, destination, fs.constants.COPYFILE_EXCL);
  }

  return [
    ...assets.map((asset) => path.join(outputDirectory, asset.name)),
    ...updateAssets.map((asset) =>
      path.join(outputDirectory, "updates", asset.platform, asset.arch, asset.name),
    ),
  ];
}

function listFiles(directory) {
  if (!fs.statSync(directory).isDirectory()) {
    throw new Error(`Forge make directory is not a directory: ${directory}`);
  }

  return fs.readdirSync(directory, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => path.join(entry.parentPath, entry.name));
}

function releaseAssetsFor(goos, goarch, files, names, packageVersion) {
  const electronArch = goarch === "amd64" ? "x64" : goarch;
  switch (`${goos}/${goarch}`) {
    case "darwin/arm64":
    case "darwin/amd64":
      return [
        asset(
          files,
          (file) => path.basename(file) === `CSGClaw-${packageVersion}-${electronArch}.dmg`,
          names[0],
        ),
        asset(
          files,
          (file) =>
            path.basename(file).endsWith(`-${packageVersion}.zip`) &&
            normalizedPath(file).includes(`/zip/darwin/${electronArch}/`),
          names[1],
        ),
      ];
    case "windows/amd64":
      return [
        asset(
          files,
          (file) => path.basename(file) === `CSGClaw-Desktop-${packageVersion}-${electronArch}-Setup.exe`,
          names[0],
        ),
      ];
    case "linux/amd64":
    case "linux/arm64":
      return [asset(files, (file) => file.endsWith(".deb"), names[0])];
    default:
      throw new Error(`unsupported desktop release target: ${goos}/${goarch}`);
  }
}

function updateAssetsFor(goos, goarch, files, packageVersion) {
  const electronArch = goarch === "amd64" ? "x64" : goarch;
  switch (`${goos}/${goarch}`) {
    case "darwin/arm64":
    case "darwin/amd64": {
      const directoryFragment = `/zip/darwin/${electronArch}/`;
      const archive = optionalAsset(
        files,
        (file) =>
          path.basename(file).endsWith(`-${packageVersion}.zip`) &&
          normalizedPath(file).includes(directoryFragment),
      );
      const manifest = optionalAsset(
        files,
        (file) => path.basename(file) === "RELEASES.json" && normalizedPath(file).includes(directoryFragment),
      );
      // The ZIP is also a public download, so its presence alone does not mean
      // this build enabled the native update feed.
      if (!manifest) {
        return [];
      }
      if (!archive) {
        throw new Error(`incomplete macOS update feed for ${goos}/${goarch}`);
      }
      return [
        { source: archive, name: path.basename(archive), platform: "darwin", arch: electronArch },
        { source: manifest, name: "RELEASES.json", platform: "darwin", arch: electronArch },
      ];
    }
    case "windows/amd64": {
      const directoryFragment = `/squirrel.windows/${electronArch}/`;
      const manifest = optionalAsset(
        files,
        (file) => path.basename(file) === "RELEASES" && normalizedPath(file).includes(directoryFragment),
      );
      const packages = files.filter(
        (file) => file.endsWith(".nupkg") && normalizedPath(file).includes(directoryFragment),
      );
      if (!manifest && packages.length === 0) {
        return [];
      }
      if (!manifest || packages.length === 0) {
        throw new Error(`incomplete Windows update feed for ${goos}/${goarch}`);
      }
      return [
        ...packages.map((file) => ({
          source: file,
          name: path.basename(file),
          platform: "win32",
          arch: electronArch,
        })),
        { source: manifest, name: "RELEASES", platform: "win32", arch: electronArch },
      ];
    }
    default:
      return [];
  }
}

function normalizedPath(file) {
  return `/${file.split(path.sep).join("/")}`;
}

function asset(files, matches, name) {
  const matchesFiles = files.filter(matches);
  if (matchesFiles.length !== 1) {
    throw new Error(`expected exactly one source for ${name}, found ${matchesFiles.length}`);
  }
  return { source: matchesFiles[0], name };
}

function optionalAsset(files, matches) {
  const matchingFiles = files.filter(matches);
  if (matchingFiles.length > 1) {
    throw new Error(`expected at most one update source, found ${matchingFiles.length}`);
  }
  return matchingFiles[0];
}

function main(args) {
  if (args.length !== 5) {
    throw new Error("usage: collect-desktop-release-assets.mjs <version> <goos> <goarch> <make-dir> <output-dir>");
  }

  const [version, goos, goarch, makeDirectory, outputDirectory] = args;
  const assets = collectDesktopReleaseAssets({ version, goos, goarch, makeDirectory, outputDirectory });
  for (const assetPath of assets) {
    console.log(`staged ${assetPath}`);
  }
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  }
}

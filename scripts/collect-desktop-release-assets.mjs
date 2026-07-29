#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

export function collectDesktopReleaseAssets({ version, goos, goarch, makeDirectory, outputDirectory }) {
  const files = listFiles(makeDirectory);
  const prefix = `csgclaw-desktop_${version}_${goos}_${goarch}`;
  const assets = releaseAssetsFor(goos, goarch, files, prefix);

  fs.mkdirSync(outputDirectory, { recursive: true });
  for (const asset of assets) {
    const destination = path.join(outputDirectory, asset.name);
    fs.copyFileSync(asset.source, destination, fs.constants.COPYFILE_EXCL);
  }

  return assets.map((asset) => path.join(outputDirectory, asset.name));
}

function listFiles(directory) {
  if (!fs.statSync(directory).isDirectory()) {
    throw new Error(`Forge make directory is not a directory: ${directory}`);
  }

  return fs.readdirSync(directory, { recursive: true, withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => path.join(entry.parentPath, entry.name));
}

function releaseAssetsFor(goos, goarch, files, prefix) {
  switch (`${goos}/${goarch}`) {
    case "darwin/arm64":
    case "darwin/amd64":
      return [
        asset(files, (file) => file.endsWith(".dmg"), `${prefix}.dmg`),
        asset(files, (file) => file.endsWith(".zip"), `${prefix}.zip`),
      ];
    case "windows/amd64":
      return [asset(files, (file) => file.endsWith("-Setup.exe"), `${prefix}.exe`)];
    case "linux/amd64":
    case "linux/arm64":
      return [asset(files, (file) => file.endsWith(".deb"), `${prefix}.deb`)];
    default:
      throw new Error(`unsupported desktop release target: ${goos}/${goarch}`);
  }
}

function asset(files, matches, name) {
  const matchesFiles = files.filter(matches);
  if (matchesFiles.length !== 1) {
    throw new Error(`expected exactly one source for ${name}, found ${matchesFiles.length}`);
  }
  return { source: matchesFiles[0], name };
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

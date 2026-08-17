import crypto from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { spawn } from "node:child_process";
import { normalizeHTTPSURL } from "./updateFeed";

export type MacChannelUpdate = {
  url: string;
  name: string;
};

export type LocalUpdateFeed = {
  url: string;
  close: () => Promise<void>;
};

export function parseMacChannelUpdate(
  payload: unknown,
  expectedVersion: string,
): MacChannelUpdate {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new Error("macOS desktop update feed must be a JSON object.");
  }
  const source = payload as Record<string, unknown>;
  const currentRelease =
    typeof source.currentRelease === "string"
      ? source.currentRelease.trim().replace(/^v/, "")
      : "";
  if (currentRelease !== expectedVersion) {
    throw new Error(
      `macOS desktop update feed latest version is ${currentRelease || "missing"}, expected ${expectedVersion}.`,
    );
  }
  if (!Array.isArray(source.releases)) {
    throw new Error("macOS desktop update feed releases are missing.");
  }
  for (const rawRelease of source.releases) {
    if (
      !rawRelease ||
      typeof rawRelease !== "object" ||
      Array.isArray(rawRelease)
    ) {
      continue;
    }
    const release = rawRelease as Record<string, unknown>;
    const version =
      typeof release.version === "string"
        ? release.version.trim().replace(/^v/, "")
        : "";
    const rawUpdate = release.updateTo;
    if (
      version !== expectedVersion ||
      !rawUpdate ||
      typeof rawUpdate !== "object" ||
      Array.isArray(rawUpdate)
    ) {
      continue;
    }
    const update = rawUpdate as Record<string, unknown>;
    const url =
      typeof update.url === "string" ? normalizeHTTPSURL(update.url) : "";
    if (!url) {
      throw new Error(
        `macOS desktop update ${expectedVersion} has an invalid archive URL.`,
      );
    }
    return {
      url,
      name:
        typeof update.name === "string" && update.name.trim()
          ? update.name.trim()
          : `CSGClaw v${expectedVersion}`,
    };
  }
  throw new Error(
    `macOS desktop update feed does not contain latest version ${expectedVersion}.`,
  );
}

export function officialMacReleaseArchiveURL(
  version: string,
  arch: string,
): string {
  if (!/^[0-9A-Za-z.+-]+$/.test(version)) {
    throw new Error(`Invalid macOS desktop release version: ${version}.`);
  }
  const goArch = arch === "arm64" ? "arm64" : arch === "x64" ? "amd64" : "";
  if (!goArch) {
    throw new Error(`Unsupported macOS desktop architecture: ${arch}.`);
  }
  const tag = `v${version}`;
  const fileName = `csgclaw-desktop_${tag}_darwin_${goArch}.zip`;
  return `https://github.com/OpenCSGs/csgclaw/releases/download/${tag}/${fileName}`;
}

export function missingSignedMacUpdatePackageError(
  channel: string,
  version: string,
  cause: unknown,
): Error {
  const detail = cause instanceof Error ? cause.message : String(cause);
  const suffix = detail.trim() ? ` ${detail.trim()}` : "";
  return new Error(
    `The ${channel} channel does not provide a signed macOS auto-update package for ${version}.${suffix}`,
  );
}

export async function startMacChannelUpdateFeed(
  update: MacChannelUpdate,
): Promise<LocalUpdateFeed> {
  const token = crypto.randomUUID();
  const responseBody = Buffer.from(
    JSON.stringify({ url: update.url, name: update.name }),
  );
  let closed = false;
  let timeout: ReturnType<typeof setTimeout> | undefined;
  const server = http.createServer((request, response) => {
    if (request.method !== "GET" || request.url !== `/${token}`) {
      response.writeHead(404, { "cache-control": "no-store" });
      response.end();
      return;
    }
    response.writeHead(200, {
      "cache-control": "no-store",
      "content-length": responseBody.length,
      "content-type": "application/json; charset=utf-8",
    });
    response.end(responseBody, () => {
      void close();
    });
  });
  const close = async (): Promise<void> => {
    if (closed) {
      return;
    }
    closed = true;
    if (timeout) {
      clearTimeout(timeout);
    }
    await new Promise<void>((resolve) => {
      server.close(() => resolve());
    });
  };
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      resolve();
    });
  });
  server.unref();
  timeout = setTimeout(() => {
    void close();
  }, 60_000);
  timeout.unref();
  const address = server.address();
  if (!address || typeof address === "string") {
    await close();
    throw new Error("Could not start the local macOS update feed.");
  }
  return {
    url: `http://127.0.0.1:${address.port}/${token}`,
    close,
  };
}

export async function downloadVerifiedArtifact(
  fetcher: (input: string, init?: { cache?: "no-store" }) => Promise<Response>,
  artifact: {
    url: string;
    sizeBytes: number;
    sha256: string;
  },
  destinationPath: string,
): Promise<void> {
  const partialPath = `${destinationPath}.part`;
  await fs.promises.mkdir(path.dirname(destinationPath), { recursive: true });
  await fs.promises.rm(partialPath, { force: true });
  const response = await fetcher(artifact.url, { cache: "no-store" });
  if (!response.ok || !response.body) {
    throw new Error(
      `Desktop installer download failed: HTTP ${response.status}.`,
    );
  }
  const file = await fs.promises.open(partialPath, "wx", 0o600);
  const hash = crypto.createHash("sha256");
  let sizeBytes = 0;
  try {
    const reader = response.body.getReader();
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      const chunk = Buffer.from(value);
      hash.update(chunk);
      sizeBytes += chunk.length;
      let offset = 0;
      while (offset < chunk.length) {
        const { bytesWritten } = await file.write(
          chunk,
          offset,
          chunk.length - offset,
          null,
        );
        if (bytesWritten === 0) {
          throw new Error("Could not write the downloaded desktop installer.");
        }
        offset += bytesWritten;
      }
    }
    await file.sync();
  } catch (error) {
    await file.close().catch(() => undefined);
    await fs.promises.rm(partialPath, { force: true });
    throw error;
  }
  await file.close();
  const sha256 = hash.digest("hex");
  if (sizeBytes !== artifact.sizeBytes || sha256 !== artifact.sha256) {
    await fs.promises.rm(partialPath, { force: true });
    throw new Error(
      "Downloaded desktop installer failed integrity verification.",
    );
  }
  await fs.promises.rm(destinationPath, { force: true });
  await fs.promises.rename(partialPath, destinationPath);
}

export async function isVerifiedArtifact(
  filePath: string,
  artifact: { sizeBytes: number; sha256: string },
): Promise<boolean> {
  try {
    const stat = await fs.promises.stat(filePath);
    if (!stat.isFile() || stat.size !== artifact.sizeBytes) {
      return false;
    }
    const hash = crypto.createHash("sha256");
    const stream = fs.createReadStream(filePath);
    for await (const chunk of stream) {
      hash.update(chunk);
    }
    return hash.digest("hex") === artifact.sha256;
  } catch {
    return false;
  }
}

export async function launchWindowsChannelInstaller(
  installerPath: string,
  updateExePath: string,
  executableName: string,
  logPath: string,
): Promise<void> {
  // Run the full installer immediately after graceful app shutdown starts.
  // Squirrel terminates any remaining package processes, replaces the app
  // root, and returns only after releasing its update lock. Relaunching after
  // that point prevents the target version (including older stable builds)
  // from checking for updates while the installer still owns the lock.
  const command = [
    `> "%CSGCLAW_CHANNEL_INSTALL_LOG%" echo installer-started`,
    `start "" /wait "%CSGCLAW_CHANNEL_INSTALLER%" --silent`,
    `if errorlevel 1 (>> "%CSGCLAW_CHANNEL_INSTALL_LOG%" echo installer-failed) else (>> "%CSGCLAW_CHANNEL_INSTALL_LOG%" echo installer-finished)`,
    `if not exist "%CSGCLAW_CHANNEL_UPDATER%" (>> "%CSGCLAW_CHANNEL_INSTALL_LOG%" echo updater-missing & exit /b 1)`,
    `start "" "%CSGCLAW_CHANNEL_UPDATER%" --processStart "%CSGCLAW_CHANNEL_EXECUTABLE%"`,
    `>> "%CSGCLAW_CHANNEL_INSTALL_LOG%" echo relaunch-requested`,
  ].join(" & ");
  const child = spawn("cmd.exe", ["/d", "/s", "/c", command], {
    detached: true,
    env: {
      ...process.env,
      CSGCLAW_CHANNEL_EXECUTABLE: executableName,
      CSGCLAW_CHANNEL_INSTALLER: installerPath,
      CSGCLAW_CHANNEL_INSTALL_LOG: logPath,
      CSGCLAW_CHANNEL_UPDATER: updateExePath,
    },
    stdio: "ignore",
    windowsHide: true,
  });
  await new Promise<void>((resolve, reject) => {
    child.once("error", reject);
    child.once("spawn", () => {
      child.off("error", reject);
      resolve();
    });
  });
  child.unref();
}

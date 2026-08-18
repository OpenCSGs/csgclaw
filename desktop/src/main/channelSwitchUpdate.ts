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

const WINDOWS_COORDINATOR_READY = "coordinator-ready";
// Leave enough time for endpoint protection to inspect the newly staged,
// signed helper before it opens the parent handle and reports readiness.
const WINDOWS_COORDINATOR_READY_TIMEOUT_MS = 10_000;
const WINDOWS_COORDINATOR_HELPER_PREFIX = "csgclaw-update-helper-";
const WINDOWS_COORDINATOR_READY_PREFIX = "channel-installer-";

export type WindowsUpdateCoordinatorInputs = {
  parentProcessId: number;
  installerPath: string;
  rootExecutablePath: string;
  readyFilePath: string;
  logPath: string;
};

export function windowsUpdateCoordinatorArguments(
  inputs: WindowsUpdateCoordinatorInputs,
): string[] {
  return [
    "--parent-pid",
    String(inputs.parentProcessId),
    "--installer",
    inputs.installerPath,
    "--root-executable",
    inputs.rootExecutablePath,
    "--ready-file",
    inputs.readyFilePath,
    "--log-file",
    inputs.logPath,
  ];
}

async function waitForWindowsCoordinatorReady(
  readyFilePath: string,
  child: ReturnType<typeof spawn>,
  logPath: string,
): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const deadline = Date.now() + WINDOWS_COORDINATOR_READY_TIMEOUT_MS;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let settled = false;

    const cleanup = (): void => {
      if (timer) {
        clearTimeout(timer);
      }
      child.off("error", fail);
      child.off("exit", exited);
    };
    const succeed = (): void => {
      if (settled) {
        return;
      }
      settled = true;
      cleanup();
      resolve();
    };
    const fail = (error: Error): void => {
      if (settled) {
        return;
      }
      settled = true;
      cleanup();
      reject(error);
    };
    const exited = (
      code: number | null,
      signal: NodeJS.Signals | null,
    ): void => {
      fail(
        new Error(
          `Windows update coordinator exited before it became ready (code=${code ?? "none"}, signal=${signal ?? "none"}). See ${logPath}.`,
        ),
      );
    };
    const poll = async (): Promise<void> => {
      try {
        const marker = await fs.promises.readFile(readyFilePath, "utf8");
        if (marker.trim() === WINDOWS_COORDINATOR_READY) {
          succeed();
          return;
        }
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
          fail(error instanceof Error ? error : new Error(String(error)));
          return;
        }
      }
      if (settled) {
        return;
      }
      if (Date.now() >= deadline) {
        fail(
          new Error(
            `Windows update coordinator did not become ready within ${WINDOWS_COORDINATOR_READY_TIMEOUT_MS / 1_000} seconds. See ${logPath}.`,
          ),
        );
        return;
      }
      timer = setTimeout(() => {
        void poll();
      }, 50);
    };

    child.once("error", fail);
    child.once("exit", exited);
    void poll();
  });
}

export async function launchWindowsChannelInstaller(
  options: Omit<WindowsUpdateCoordinatorInputs, "readyFilePath"> & {
    helperResourcePath: string;
  },
): Promise<void> {
  // Stage the signed, no-console Windows helper outside the installed app
  // directory. It opens a handle to this exact Electron process before
  // signaling readiness, then uses the Windows wait API rather than parsing
  // tasklist output. The full installer can therefore replace the old app
  // directory without locking the helper executable that coordinates it.
  const coordinatorDirectory = path.dirname(options.installerPath);
  await fs.promises.mkdir(coordinatorDirectory, { recursive: true });
  await fs.promises.mkdir(path.dirname(options.logPath), { recursive: true });
  await removeStaleWindowsCoordinatorArtifacts(coordinatorDirectory);

  const attemptId = crypto.randomUUID();
  const helperPath = path.join(
    coordinatorDirectory,
    `${WINDOWS_COORDINATOR_HELPER_PREFIX}${attemptId}.exe`,
  );
  const readyFilePath = path.join(
    coordinatorDirectory,
    `${WINDOWS_COORDINATOR_READY_PREFIX}${attemptId}.ready`,
  );
  const partialHelperPath = `${helperPath}.part`;
  try {
    await fs.promises.copyFile(options.helperResourcePath, partialHelperPath);
    await fs.promises.rename(partialHelperPath, helperPath);
  } catch (error) {
    await fs.promises.rm(partialHelperPath, { force: true });
    throw new Error(
      `Could not stage the Windows update coordinator: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  await fs.promises.writeFile(options.logPath, "", {
    encoding: "utf8",
    mode: 0o600,
  });

  const child = spawn(
    helperPath,
    windowsUpdateCoordinatorArguments({
      installerPath: options.installerPath,
      logPath: options.logPath,
      parentProcessId: options.parentProcessId,
      readyFilePath,
      rootExecutablePath: options.rootExecutablePath,
    }),
    {
      cwd: coordinatorDirectory,
      detached: true,
      stdio: "ignore",
      windowsHide: true,
    },
  );

  try {
    await waitForWindowsCoordinatorReady(readyFilePath, child, options.logPath);
  } catch (error) {
    try {
      child.kill();
    } catch {
      // Preserve the coordinator readiness error if the process already exited.
    }
    throw error;
  }
  child.unref();
}

async function removeStaleWindowsCoordinatorArtifacts(
  directory: string,
): Promise<void> {
  const entries = await fs.promises.readdir(directory, { withFileTypes: true });
  await Promise.all(
    entries
      .filter(
        (entry) =>
          entry.isFile() &&
          (entry.name === "channel-installer.cmd" ||
            entry.name === "channel-installer.ready" ||
            entry.name.startsWith(WINDOWS_COORDINATOR_HELPER_PREFIX) ||
            (entry.name.startsWith(WINDOWS_COORDINATOR_READY_PREFIX) &&
              entry.name.endsWith(".ready"))),
      )
      .map(async (entry) => {
        try {
          await fs.promises.rm(path.join(directory, entry.name), {
            force: true,
          });
        } catch {
          // A still-running helper is locked on Windows. A unique filename lets
          // the new attempt proceed while preserving that process for diagnosis.
        }
      }),
  );
}

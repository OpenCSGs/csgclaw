#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { setTimeout as sleep } from "node:timers/promises";
import { pathToFileURL } from "node:url";

const retryDelays = [30_000, 90_000];
const commandTimeout = 180_000;
const networkError =
  /NSURLErrorDomain\s+Code=-(?:1001|1003|1004|1005|1006|1009)\b|network connection was lost|network is (?:down|unreachable)|not connected to the internet|connection (?:reset|refused)|could not (?:connect|resolve)|hostname could not be found|timed? out/i;
const permanentError =
  /(?:invalid|revoked).*(?:signature|ticket)|(?:signature|ticket).*(?:invalid|revoked)|not signed|authentication failed|unauthorized|forbidden/i;

// Retry the existing artifact, never rebuilding, signing, or resubmitting it.
export async function runStapler(
  action,
  artifactPath,
  {
    environment = process.env,
    execute = spawnSync,
    wait = sleep,
    log = console.log,
  } = {},
) {
  if (!["staple", "validate"].includes(action) || !artifactPath) {
    throw new Error(
      "usage: macos-stapler.mjs <staple|validate> <app-or-dmg-path>",
    );
  }
  const attempts = retryDelays.length + 1;
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    log(`[stapler ${action}] attempt ${attempt}/${attempts}: ${artifactPath}`);
    const result = execute("xcrun", ["stapler", action, artifactPath], {
      env: environment,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
      timeout: commandTimeout,
      killSignal: "SIGKILL",
      maxBuffer: 4 * 1024 * 1024,
    });
    const output = [result.stdout, result.stderr]
      .filter(Boolean)
      .join("\n")
      .trim();
    if (output) log(output);
    if (!result.error && result.status === 0) {
      log(`[stapler ${action}] succeeded on attempt ${attempt}/${attempts}`);
      return;
    }

    const timedOut = result.error?.code === "ETIMEDOUT";
    const interrupted = result.signal && !timedOut;
    const transient =
      !interrupted &&
      !permanentError.test(output) &&
      (timedOut || (!result.error && networkError.test(output)));
    const reason = timedOut
      ? "command timed out after 180 seconds"
      : result.error?.message ||
        (result.signal
          ? `interrupted by ${result.signal}`
          : `exit code ${result.status}`);
    if (!transient || attempt === attempts) {
      const error = new Error(
        `[stapler ${action}] failed on attempt ${attempt}/${attempts}: ${reason}; ` +
          (transient
            ? "network retry limit reached"
            : "not a transient network error"),
      );
      error.exitCode =
        Number.isInteger(result.status) &&
        result.status > 0 &&
        result.status < 256
          ? result.status
          : 1;
      throw error;
    }
    const delay = retryDelays[attempt - 1];
    log(
      `[stapler ${action}] transient network failure (${reason}); retrying the same artifact in ${delay / 1000} seconds`,
    );
    await wait(delay);
  }
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  try {
    if (process.argv.length !== 4)
      throw new Error(
        "usage: macos-stapler.mjs <staple|validate> <app-or-dmg-path>",
      );
    await runStapler(process.argv[2], process.argv[3]);
  } catch (error) {
    console.error(error.message);
    process.exitCode = error.exitCode || 1;
  }
}

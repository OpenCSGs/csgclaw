import { execFile } from "node:child_process";
import { userInfo } from "node:os";
import path from "node:path";
import { resolveSystemProxyEnvironment, type SystemProxyResolver } from "./systemProxy";

const ENVIRONMENT_CAPTURE_TIMEOUT_MS = 5_000;
const ENVIRONMENT_CAPTURE_MAX_BYTES = 1024 * 1024;
const POSIX_ENVIRONMENT_COMMAND = "/usr/bin/env -0";
const SHELL_ENVIRONMENT_KEYS = [
  "PATH",
  "CSGCLAW_CODEX_PATH",
  "CSGCLAW_CODEX_ACP_PATH",
  "DOCKER_HOST",
  "DOCKER_CONTEXT",
  "DOCKER_CONFIG",
  "DOCKER_CERT_PATH",
  "DOCKER_TLS_VERIFY",
] as const;
const WINDOWS_ENVIRONMENT_SCRIPT = `
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
$names = @(${SHELL_ENVIRONMENT_KEYS.map((key) => `'${key}'`).join(", ")})
$result = @{}
foreach ($name in $names) {
  $machine = [Environment]::GetEnvironmentVariable($name, 'Machine')
  $user = [Environment]::GetEnvironmentVariable($name, 'User')
  if ($name -eq 'PATH') {
    $parts = @($machine, $user) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    if ($parts.Count -gt 0) {
      $result[$name] = [Environment]::ExpandEnvironmentVariables(($parts -join ';'))
    }
  } elseif (-not [string]::IsNullOrWhiteSpace($user)) {
    $result[$name] = [Environment]::ExpandEnvironmentVariables($user)
  } elseif (-not [string]::IsNullOrWhiteSpace($machine)) {
    $result[$name] = [Environment]::ExpandEnvironmentVariables($machine)
  }
}
[Console]::Out.Write(($result | ConvertTo-Json -Compress))
`.trim();

export type EnvironmentCommandOptions = {
  env: NodeJS.ProcessEnv;
  maxBuffer: number;
  timeout: number;
  windowsHide: boolean;
};

export type EnvironmentCommandRunner = (
  executable: string,
  args: readonly string[],
  options: EnvironmentCommandOptions,
) => Promise<string>;

export type ResolveSidecarEnvironmentOptions = {
  baseEnvironment?: NodeJS.ProcessEnv;
  homeDirectory: string;
  loginShell?: string;
  platform?: NodeJS.Platform;
  runCommand?: EnvironmentCommandRunner;
  resolveSystemProxy?: SystemProxyResolver;
};

export async function resolveSidecarEnvironment({
  baseEnvironment = process.env,
  homeDirectory,
  loginShell,
  platform = process.platform,
  runCommand = runEnvironmentCommand,
  resolveSystemProxy,
}: ResolveSidecarEnvironmentOptions): Promise<NodeJS.ProcessEnv> {
  // Keep executable discovery in the Go server unchanged. Electron only
  // restores the environment that a newly opened terminal would normally
  // provide before it starts the same server entrypoint.
  const env = sanitizeEnvironment(baseEnvironment, platform);
  const currentPath = environmentValue(env, "PATH", platform);
  let discovered: NodeJS.ProcessEnv = {};

  try {
    if (platform === "darwin" || platform === "linux") {
      const shell = resolveLoginShell(env, loginShell, platform);
      const output = await runCommand(
        shell,
        terminalShellArguments(shell, platform),
        commandOptions(env),
      );
      discovered = parseNullSeparatedEnvironment(output);
    } else if (platform === "win32") {
      const powershell = resolveWindowsPowerShell(env);
      const output = await runCommand(
        powershell,
        [
          "-NoLogo",
          "-NoProfile",
          "-NonInteractive",
          "-Command",
          WINDOWS_ENVIRONMENT_SCRIPT,
        ],
        commandOptions(env),
      );
      discovered = parseWindowsEnvironment(output);
    }
  } catch {
    // A customized shell can be slow, interactive, or broken. Starting the
    // sidecar with Electron's inherited environment is safer than blocking the
    // whole desktop application.
  }

  const discoveredPath = environmentValue(discovered, "PATH", platform);
  setEnvironmentValue(
    env,
    "PATH",
    mergeExecutableSearchPath(
      discoveredPath,
      [currentPath, ...platformPathFallbacks(platform, homeDirectory)],
      platform,
    ),
    platform,
  );

  for (const key of SHELL_ENVIRONMENT_KEYS) {
    if (key === "PATH" || environmentValue(env, key, platform)?.trim()) {
      continue;
    }
    const value = environmentValue(discovered, key, platform)?.trim();
    if (value) {
      setEnvironmentValue(env, key, value, platform);
    }
  }

  if (resolveSystemProxy) {
    const systemProxy = await resolveSystemProxyEnvironment(resolveSystemProxy);
    for (const [key, value] of Object.entries(systemProxy)) {
      if (value && !environmentValue(env, key, platform)?.trim()) {
        setEnvironmentValue(env, key, value, platform);
      }
    }
  }

  return env;
}

export function parseNullSeparatedEnvironment(
  output: string,
): NodeJS.ProcessEnv {
  const result: NodeJS.ProcessEnv = {};

  for (const record of output.split("\0")) {
    for (const key of SHELL_ENVIRONMENT_KEYS) {
      const marker = `${key}=`;
      const markerIndex = record.indexOf(marker);
      if (
        markerIndex < 0 ||
        (markerIndex > 0 &&
          record[markerIndex - 1] !== "\n" &&
          record[markerIndex - 1] !== "\r")
      ) {
        continue;
      }
      result[key] = record.slice(markerIndex + marker.length);
      break;
    }
  }

  return result;
}

export function mergeExecutableSearchPath(
  primary: string | undefined,
  fallbacks: Array<string | undefined>,
  platform: NodeJS.Platform,
): string {
  const delimiter =
    platform === "win32" ? path.win32.delimiter : path.posix.delimiter;
  const seen = new Set<string>();
  const entries: string[] = [];

  for (const value of [primary, ...fallbacks]) {
    for (const rawEntry of (value || "").split(delimiter)) {
      const entry = rawEntry.trim();
      if (!entry) {
        continue;
      }
      const comparisonKey = platform === "win32" ? entry.toLowerCase() : entry;
      if (seen.has(comparisonKey)) {
        continue;
      }
      seen.add(comparisonKey);
      entries.push(entry);
    }
  }

  return entries.join(delimiter);
}

function sanitizeEnvironment(
  baseEnvironment: NodeJS.ProcessEnv,
  platform: NodeJS.Platform,
): NodeJS.ProcessEnv {
  const env = { ...baseEnvironment };
  deleteEnvironmentValue(env, "ELECTRON_RUN_AS_NODE", platform);
  deleteEnvironmentValue(env, "NODE_OPTIONS", platform);
  return env;
}

function resolveLoginShell(
  env: NodeJS.ProcessEnv,
  loginShell: string | undefined,
  platform: NodeJS.Platform,
): string {
  const configured =
    environmentValue(env, "SHELL", platform)?.trim() ||
    loginShell?.trim() ||
    operatingSystemLoginShell();
  if (configured && path.posix.isAbsolute(configured)) {
    return configured;
  }
  return platform === "darwin" ? "/bin/zsh" : "/bin/sh";
}

function operatingSystemLoginShell(): string | undefined {
  try {
    return userInfo().shell?.trim() || undefined;
  } catch {
    return undefined;
  }
}

function terminalShellArguments(
  shell: string,
  platform: NodeJS.Platform,
): string[] {
  const shellName = path.posix.basename(shell).toLowerCase();
  const useLoginShell = platform === "darwin";
  if (shellName === "fish") {
    return [
      ...(useLoginShell ? ["--login"] : []),
      "--interactive",
      "--command",
      POSIX_ENVIRONMENT_COMMAND,
    ];
  }
  if (shellName === "csh" || shellName === "tcsh") {
    return [...(useLoginShell ? ["-l"] : []), "-c", POSIX_ENVIRONMENT_COMMAND];
  }
  return [useLoginShell ? "-ilc" : "-ic", POSIX_ENVIRONMENT_COMMAND];
}

function resolveWindowsPowerShell(env: NodeJS.ProcessEnv): string {
  const systemRoot = environmentValue(env, "SystemRoot", "win32")?.trim();
  if (!systemRoot) {
    return "powershell.exe";
  }
  return path.win32.join(
    systemRoot,
    "System32",
    "WindowsPowerShell",
    "v1.0",
    "powershell.exe",
  );
}

function parseWindowsEnvironment(output: string): NodeJS.ProcessEnv {
  const parsed: unknown = JSON.parse(output.trim());
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    return {};
  }

  const result: NodeJS.ProcessEnv = {};
  for (const key of SHELL_ENVIRONMENT_KEYS) {
    const value = (parsed as Record<string, unknown>)[key];
    if (typeof value === "string" && value.trim()) {
      result[key] = value;
    }
  }
  return result;
}

function commandOptions(env: NodeJS.ProcessEnv): EnvironmentCommandOptions {
  return {
    env,
    maxBuffer: ENVIRONMENT_CAPTURE_MAX_BYTES,
    timeout: ENVIRONMENT_CAPTURE_TIMEOUT_MS,
    windowsHide: true,
  };
}

function platformPathFallbacks(
  platform: NodeJS.Platform,
  homeDirectory: string,
): string[] {
  if (platform === "darwin") {
    return [
      path.join(homeDirectory, ".docker", "bin"),
      "/opt/homebrew/bin",
      "/usr/local/bin",
      "/Applications/Docker.app/Contents/Resources/bin",
      "/usr/bin",
      "/bin",
      "/usr/sbin",
      "/sbin",
    ];
  }
  if (platform === "linux") {
    return [
      path.join(homeDirectory, ".local", "bin"),
      "/usr/local/bin",
      "/usr/bin",
      "/bin",
      "/usr/sbin",
      "/sbin",
    ];
  }
  return [];
}

function environmentValue(
  env: NodeJS.ProcessEnv,
  key: string,
  platform: NodeJS.Platform,
): string | undefined {
  if (platform !== "win32") {
    return env[key];
  }
  const existingKey = Object.keys(env).find(
    (candidate) => candidate.toLowerCase() === key.toLowerCase(),
  );
  return existingKey ? env[existingKey] : undefined;
}

function setEnvironmentValue(
  env: NodeJS.ProcessEnv,
  key: string,
  value: string,
  platform: NodeJS.Platform,
): void {
  if (platform !== "win32") {
    env[key] = value;
    return;
  }
  const existingKey = Object.keys(env).find(
    (candidate) => candidate.toLowerCase() === key.toLowerCase(),
  );
  env[existingKey || (key === "PATH" ? "Path" : key)] = value;
}

function deleteEnvironmentValue(
  env: NodeJS.ProcessEnv,
  key: string,
  platform: NodeJS.Platform,
): void {
  if (platform !== "win32") {
    delete env[key];
    return;
  }
  for (const existingKey of Object.keys(env)) {
    if (existingKey.toLowerCase() === key.toLowerCase()) {
      delete env[existingKey];
    }
  }
}

function runEnvironmentCommand(
  executable: string,
  args: readonly string[],
  options: EnvironmentCommandOptions,
): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile(
      executable,
      [...args],
      { ...options, encoding: "utf8" },
      (error, stdout) => {
        if (error) {
          reject(error);
          return;
        }
        resolve(stdout);
      },
    );
  });
}

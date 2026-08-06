import assert from "node:assert/strict";
import test from "node:test";
import { DesktopPlatform } from "../../shared/desktopEnvironment";
import {
  mergeExecutableSearchPath,
  parseNullSeparatedEnvironment,
  resolveSidecarEnvironment,
  type EnvironmentCommandRunner,
} from "./childEnvironment";

test("uses the login shell PATH before Electron's inherited macOS PATH", async () => {
  const runCommand: EnvironmentCommandRunner = async (executable, args) => {
    assert.equal(executable, "/bin/zsh");
    assert.deepEqual(args, ["-ilc", "/usr/bin/env -0"]);
    return [
      "shell startup notice\nPATH=/Users/test/.bun/bin:/opt/homebrew/bin",
      "CSGCLAW_CODEX_PATH=/Users/test/bin/codex",
      "UNRELATED_SECRET=not-imported",
      "",
    ].join("\0");
  };

  const env = await resolveSidecarEnvironment({
    baseEnvironment: {
      ELECTRON_RUN_AS_NODE: "1",
      NODE_OPTIONS: "--require electron",
      PATH: "/usr/bin:/bin",
    },
    homeDirectory: "/Users/test",
    loginShell: "/bin/zsh",
    platform: DesktopPlatform.MacOS,
    runCommand,
  });

  assert.equal(
    env.PATH,
    [
      "/Users/test/.bun/bin",
      "/opt/homebrew/bin",
      "/usr/bin",
      "/bin",
      "/Users/test/.docker/bin",
      "/usr/local/bin",
      "/Applications/Docker.app/Contents/Resources/bin",
      "/usr/sbin",
      "/sbin",
    ].join(":"),
  );
  assert.equal(env.CSGCLAW_CODEX_PATH, undefined);
  assert.equal(env.UNRELATED_SECRET, undefined);
  assert.equal(env.ELECTRON_RUN_AS_NODE, undefined);
  assert.equal(env.NODE_OPTIONS, undefined);
});

test("keeps the inherited environment when login shell discovery fails", async () => {
  const env = await resolveSidecarEnvironment({
    baseEnvironment: { PATH: "/custom/bin:/usr/bin" },
    homeDirectory: "/home/test",
    loginShell: "/bin/bash",
    platform: DesktopPlatform.Linux,
    runCommand: async () => {
      throw new Error("shell startup timed out");
    },
  });

  assert.equal(
    env.PATH,
    [
      "/custom/bin",
      "/usr/bin",
      "/home/test/.local/bin",
      "/usr/local/bin",
      "/bin",
      "/usr/sbin",
      "/sbin",
    ].join(":"),
  );
});

test("scopes the system proxy to embedded CLIProxy without importing shell proxy settings", async () => {
  const env = await resolveSidecarEnvironment({
    baseEnvironment: { PATH: "/usr/bin:/bin" },
    homeDirectory: "/Users/test",
    loginShell: "/bin/zsh",
    platform: DesktopPlatform.MacOS,
    runCommand: async () =>
      [
        "PATH=/opt/homebrew/bin:/usr/bin",
        "https_proxy=http://127.0.0.1:7890",
        "UNRELATED_SECRET=not-imported",
        "",
      ].join("\0"),
    resolveSystemProxy: async () => "PROXY 127.0.0.1:7890",
  });

  assert.equal(env.CSGCLAW_CLIPROXY_SYSTEM_PROXY_URL, "http://127.0.0.1:7890");
  assert.equal(env.HTTP_PROXY, undefined);
  assert.equal(env.HTTPS_PROXY, undefined);
  assert.equal(env.http_proxy, undefined);
  assert.equal(env.https_proxy, undefined);
  assert.equal(env.NO_PROXY, undefined);
  assert.equal(env.no_proxy, undefined);
  assert.equal(env.UNRELATED_SECRET, undefined);
});

test("scopes an inherited standard proxy to embedded CLIProxy", async () => {
  const env = await resolveSidecarEnvironment({
    baseEnvironment: {
      PATH: "/usr/bin:/bin",
      HTTPS_PROXY: "http://127.0.0.1:7891",
      HTTP_PROXY: "http://127.0.0.1:7892",
    },
    homeDirectory: "/home/test",
    loginShell: "/bin/bash",
    platform: DesktopPlatform.Linux,
    runCommand: async () => "PATH=/usr/bin:/bin\0",
    resolveSystemProxy: async () => "PROXY 127.0.0.1:7890",
  });

  assert.equal(env.CSGCLAW_CLIPROXY_SYSTEM_PROXY_URL, "http://127.0.0.1:7891");
  assert.equal(env.HTTPS_PROXY, undefined);
  assert.equal(env.HTTP_PROXY, undefined);
});

test("uses an interactive non-login shell for Linux terminal configuration", async () => {
  const runCommand: EnvironmentCommandRunner = async (executable, args) => {
    assert.equal(executable, "/bin/bash");
    assert.deepEqual(args, ["-ic", "/usr/bin/env -0"]);
    return "PATH=/home/test/.local/bin:/usr/bin\0";
  };

  const env = await resolveSidecarEnvironment({
    baseEnvironment: { PATH: "/usr/bin:/bin" },
    homeDirectory: "/home/test",
    loginShell: "/bin/bash",
    platform: DesktopPlatform.Linux,
    runCommand,
  });

  assert.equal(
    env.PATH,
    [
      "/home/test/.local/bin",
      "/usr/bin",
      "/bin",
      "/usr/local/bin",
      "/usr/sbin",
      "/sbin",
    ].join(":"),
  );
});

test("uses Windows user and machine environment while ignoring Codex path overrides", async () => {
  const runCommand: EnvironmentCommandRunner = async (executable, args) => {
    assert.equal(
      executable,
      "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
    );
    assert.deepEqual(args.slice(0, 4), [
      "-NoLogo",
      "-NoProfile",
      "-NonInteractive",
      "-Command",
    ]);
    return JSON.stringify({
      PATH: "C:\\Windows\\System32;C:\\Users\\test\\AppData\\Roaming\\npm",
      CSGCLAW_CODEX_PATH: "C:\\Users\\test\\AppData\\Roaming\\npm\\codex.cmd",
    });
  };

  const env = await resolveSidecarEnvironment({
    baseEnvironment: {
      Node_Options: "--require electron",
      Path: "C:\\Windows\\System32;C:\\Tools",
      CSGCLAW_CODEX_PATH: "D:\\codex\\codex.exe",
      SystemRoot: "C:\\Windows",
    },
    executableSearchPathEntries: [
      "C:\\Program Files\\CSGClaw\\resources\\backend\\csgclaw\\bin",
    ],
    homeDirectory: "C:\\Users\\test",
    platform: DesktopPlatform.Windows,
    runCommand,
  });

  assert.equal(
    env.Path,
    [
      "C:\\Program Files\\CSGClaw\\resources\\backend\\csgclaw\\bin",
      "C:\\Windows\\System32",
      "C:\\Users\\test\\AppData\\Roaming\\npm",
      "C:\\Tools",
    ].join(";"),
  );
  assert.equal(env.CSGCLAW_CODEX_PATH, undefined);
  assert.equal(env.Node_Options, undefined);
});

test("parses only allowlisted values from null-separated shell output", () => {
  assert.deepEqual(
    parseNullSeparatedEnvironment(
      "shell_notice=value\nPATH=/shell/bin\0DOCKER_HOST=unix:///tmp/docker.sock\0HTTPS_PROXY=http://127.0.0.1:7890\0TOKEN=secret\0",
    ),
    {
      PATH: "/shell/bin",
      DOCKER_HOST: "unix:///tmp/docker.sock",
    },
  );
});

test("deduplicates Windows PATH entries case-insensitively", () => {
  assert.equal(
    mergeExecutableSearchPath(
      ["C:\\Tools", "C:\\Windows"],
      ["c:\\tools;D:\\Bin"],
      DesktopPlatform.Windows,
    ),
    "C:\\Tools;C:\\Windows;D:\\Bin",
  );
});

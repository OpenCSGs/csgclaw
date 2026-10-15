const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
if ((process.env.CSGCLAW_DESKTOP_GOOS || process.platform) !== "darwin")
  process.exit(0);
if (process.platform !== "darwin")
  throw new Error("Build the macOS notification helper on macOS.");
const desktopRoot = path.resolve(__dirname, "..");
const index = process.argv.indexOf("--out-dir");
const output =
  index < 0
    ? path.join(desktopRoot, "dist", "native")
    : path.resolve(process.argv[index + 1]);
const helper = path.join(output, "CSGClaw Notifications.app");
const executable = path.join(
  helper,
  "Contents",
  "MacOS",
  "CSGClawNotifications",
);
const arch = { arm64: "arm64", x64: "x86_64", amd64: "x86_64" }[
  process.env.CSGCLAW_DESKTOP_ARCH || process.env.CSGCLAW_DESKTOP_GOARCH || process.arch
];
if (!arch)
  throw new Error("Unsupported macOS notification helper architecture.");
fs.mkdirSync(path.dirname(executable), { recursive: true });
fs.mkdirSync(path.join(helper, "Contents", "Resources"), { recursive: true });
fs.writeFileSync(
  path.join(helper, "Contents", "Info.plist"),
  `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleExecutable</key><string>CSGClawNotifications</string><key>CFBundleIdentifier</key><string>com.opencsg.csgclaw.desktop.notifications</string><key>CFBundleName</key><string>CSGClaw Notifications</string><key>CFBundleDisplayName</key><string>CSGClaw 通知</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleVersion</key><string>1</string><key>CFBundleIconFile</key><string>csgclaw.icns</string><key>LSUIElement</key><true/></dict></plist>`,
);
fs.copyFileSync(
  path.join(desktopRoot, "resources", "icons", "csgclaw-theme.icns"),
  path.join(helper, "Contents", "Resources", "csgclaw.icns"),
);
const run = (command, args) => {
  const result = spawnSync(command, args, { stdio: "inherit" });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status || 1);
};
run("xcrun", [
  "clang",
  "-fobjc-arc",
  "-arch",
  arch,
  "-mmacosx-version-min=11.0",
  "-framework",
  "Cocoa",
  "-framework",
  "UserNotifications",
  path.join(desktopRoot, "native", "macos", "notificationHelper.m"),
  "-o",
  executable,
]);
// Forge applies the distribution identity when signing the enclosing app.
// Ad-hoc signing also makes the helper usable by the local development build.
run("codesign", ["--force", "--sign", "-", helper]);

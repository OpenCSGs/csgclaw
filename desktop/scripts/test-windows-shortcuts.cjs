// Run after the desktop TypeScript build with Electron on Windows:
// pnpm exec electron scripts/test-windows-shortcuts.cjs
// All shortcut writes are confined to an isolated temporary installation.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { app, shell, nativeImage } = require("electron");
const { syncWindowsShortcutIcon } = require("../dist/main/windowsShortcuts");
const { windowsAppUserModelID } = require("../dist/main/windowsTaskbar");

app
  .whenReady()
  .then(() => {
    assert.equal(
      process.platform,
      "win32",
      "This integration test requires Windows",
    );
    const root = fs.mkdtempSync(
      path.join(os.tmpdir(), "csgclaw-native-shortcuts-"),
    );
    try {
      const installRoot = path.join(root, "CSGClaw install");
      const desktop = path.join(root, "Desktop");
      fs.mkdirSync(installRoot);
      fs.mkdirSync(desktop);
      const target = path.join(installRoot, "Update.exe");
      fs.writeFileSync(target, "test fixture; never executed");
      const shortcutPath = path.join(desktop, "CSGClaw.lnk");
      assert.equal(
        shell.writeShortcutLink(shortcutPath, "create", {
          target,
          args: '--processStart "CSGClaw.exe"',
          cwd: installRoot,
          description: "CSGClaw integration fixture",
          appUserModelId: windowsAppUserModelID,
          icon: target,
          iconIndex: 0,
        }),
        true,
      );
      const original = shell.readShortcutLink(shortcutPath);
      const errors = [];
      const options = {
        platform: "win32",
        windowsStore: false,
        packaged: true,
        executablePath: path.join(installRoot, "app-1.0.0", "CSGClaw.exe"),
        appData: path.join(root, "Roaming"),
        userData: path.join(root, "UserData"),
        desktop,
        shell,
        onError: (error) => errors.push(error),
      };
      const savedIcons = [];
      for (const [version, theme] of [
        ["1.0.0", "light"],
        ["2.0.0", "dark"],
        ["3.0.0", "light"],
      ]) {
        options.executablePath = path.join(
          installRoot,
          `app-${version}`,
          "CSGClaw.exe",
        );
        const iconPath = path.resolve(
          __dirname,
          "..",
          "resources",
          "icons",
          `csgclaw-taskbar-${theme}.ico`,
        );
        assert.equal(nativeImage.createFromPath(iconPath).isEmpty(), false);
        const persistentIcon = syncWindowsShortcutIcon({
          ...options,
          iconPath,
        });
        const updated = shell.readShortcutLink(shortcutPath);
        assert.deepEqual(updated, {
          ...original,
          icon: persistentIcon,
          iconIndex: 0,
        });
        assert.equal(
          nativeImage.createFromPath(persistentIcon).isEmpty(),
          false,
        );
        savedIcons.push(persistentIcon);
      }
      assert.notEqual(savedIcons[0], savedIcons[1]);
      assert.equal(savedIcons[0], savedIcons[2]);
      for (const icon of savedIcons) assert.equal(fs.existsSync(icon), true);
      assert.deepEqual(errors, []);
      console.log(
        "PASS: native shortcut light/dark/light icon updates preserve all launch and identity fields across two version changes.",
      );
    } finally {
      assert.equal(path.dirname(root), path.resolve(os.tmpdir()));
      fs.rmSync(root, { recursive: true, force: true });
    }
    app.exit(0);
  })
  .catch((error) => {
    console.error(error);
    app.exit(1);
  });

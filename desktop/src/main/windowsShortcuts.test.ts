import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test, { type TestContext } from "node:test";
import type { ShortcutDetails } from "electron";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import { windowsAppUserModelID } from "./windowsTaskbar";
import {
  isInstalledSquirrelShortcut,
  syncWindowsShortcutIcon,
  type WindowsShortcutIconOptions,
} from "./windowsShortcuts";

function fixture(t: TestContext) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "csgclaw-shortcut-test-"));
  t.after(() => {
    assert.equal(path.dirname(root), path.resolve(os.tmpdir()));
    fs.rmSync(root, { recursive: true, force: true });
  });
  const installRoot = path.join(root, "install");
  fs.mkdirSync(installRoot);
  fs.writeFileSync(path.join(installRoot, "Update.exe"), "fixture");
  const iconPath = path.join(root, "source.ico");
  fs.writeFileSync(iconPath, "light icon bytes");
  const shortcuts = new Map<string, ShortcutDetails>();
  const writes: { file: string; fields: ShortcutDetails }[] = [];
  const errors: unknown[] = [];
  const options: WindowsShortcutIconOptions = {
    platform: DesktopPlatform.Windows,
    windowsStore: false,
    packaged: true,
    executablePath: path.join(installRoot, "app-1.0.0", "CSGClaw.exe"),
    appData: path.join(root, "roaming"),
    userData: path.join(root, "user-data"),
    desktop: path.join(root, "desktop"),
    iconPath,
    shell: {
      readShortcutLink(file) {
        const shortcut = shortcuts.get(file);
        if (!shortcut) throw new Error("Invalid shortcut");
        return shortcut;
      },
      writeShortcutLink(file, operation, fields) {
        assert.equal(operation, "update");
        assert.deepEqual(Object.keys(fields).sort(), ["icon", "iconIndex"]);
        writes.push({ file, fields });
        shortcuts.set(file, { ...shortcuts.get(file)!, ...fields });
        return true;
      },
    },
    onError: (error) => errors.push(error),
  };
  function add(
    directory: string,
    name: string,
    overrides: Partial<ShortcutDetails> = {},
  ) {
    fs.mkdirSync(directory, { recursive: true });
    const file = path.join(directory, name);
    fs.writeFileSync(file, "fixture");
    shortcuts.set(file, {
      target: path.join(installRoot, "Update.exe"),
      args: '--processStart "CSGClaw.exe"',
      cwd: installRoot,
      description: "CSGClaw",
      appUserModelId: windowsAppUserModelID,
      icon: "installed.ico",
      iconIndex: 0,
      ...overrides,
    });
    return file;
  }
  return { options, installRoot, shortcuts, writes, errors, add };
}

test("updates full shortcut icons for this installation and preserves every launch field", (t) => {
  const f = fixture(t);
  const pinned = path.join(
    f.options.appData,
    "Microsoft",
    "Internet Explorer",
    "Quick Launch",
    "User Pinned",
    "TaskBar",
  );
  const own = f.add(pinned, "CSGClaw.lnk");
  const startMenu = f.add(
    path.join(
      f.options.appData,
      "Microsoft",
      "Windows",
      "Start Menu",
      "Programs",
      "OpenCSG",
    ),
    "CSGClaw.lnk",
  );
  const otherApp = f.add(pinned, "other.lnk", { appUserModelId: "other.app" });
  const otherInstall = f.add(pinned, "other-install.lnk", {
    target: "C:\\other\\Update.exe",
  });
  const versioned = f.add(pinned, "legacy.lnk", {
    target: f.options.executablePath,
  });
  const before = new Map(f.shortcuts);
  const icon = syncWindowsShortcutIcon(f.options);
  assert.equal(fs.readFileSync(icon, "utf8"), "light icon bytes");
  assert.equal(
    path.dirname(icon),
    path.join(f.options.userData, "taskbar-icons"),
  );
  assert.deepEqual(
    new Set(f.writes.map((write) => write.file)),
    new Set([own, startMenu]),
  );
  for (const file of [own, startMenu]) {
    assert.deepEqual(f.shortcuts.get(file), {
      ...before.get(file),
      icon,
      iconIndex: 0,
    });
  }
  for (const file of [otherApp, otherInstall, versioned])
    assert.deepEqual(f.shortcuts.get(file), before.get(file));
  syncWindowsShortcutIcon(f.options);
  assert.equal(f.writes.length, 2);
  assert.deepEqual(f.errors, []);
});

test("keeps prior icon resources valid through two app upgrades and switching back", (t) => {
  const f = fixture(t);
  f.add(f.options.desktop, "CSGClaw.lnk");
  const light = syncWindowsShortcutIcon(f.options);
  fs.writeFileSync(f.options.iconPath, "dark icon bytes");
  f.options.executablePath = path.join(
    f.installRoot,
    "app-2.0.0",
    "CSGClaw.exe",
  );
  const dark = syncWindowsShortcutIcon(f.options);
  assert.notEqual(light, dark);
  assert.equal(fs.readFileSync(light, "utf8"), "light icon bytes");
  assert.equal(fs.readFileSync(dark, "utf8"), "dark icon bytes");
  fs.writeFileSync(f.options.iconPath, "light icon bytes");
  f.options.executablePath = path.join(
    f.installRoot,
    "app-3.0.0",
    "CSGClaw.exe",
  );
  assert.equal(syncWindowsShortcutIcon(f.options), light);
  assert.equal(f.writes.length, 3);
});

test("does not touch shortcuts or cache icons for MSIX, development, or non-Windows builds", (t) => {
  const f = fixture(t);
  for (const overrides of [
    { windowsStore: true },
    { packaged: false },
    { platform: DesktopPlatform.MacOS },
    { executablePath: path.join(f.installRoot, "CSGClaw.exe") },
  ]) {
    assert.equal(
      syncWindowsShortcutIcon({ ...f.options, ...overrides }),
      f.options.iconPath,
    );
  }
  assert.equal(fs.existsSync(f.options.userData), false);
  assert.deepEqual(f.writes, []);
});

test("ignores broken links, reports failed writes, and continues updating other owned links", (t) => {
  const f = fixture(t);
  const broken = f.add(f.options.desktop, "a-broken.lnk");
  f.shortcuts.delete(broken);
  const locked = f.add(f.options.desktop, "b-locked.lnk");
  const good = f.add(f.options.desktop, "c-good.lnk");
  const write = f.options.shell.writeShortcutLink;
  f.options.shell.writeShortcutLink = (file, operation, details) =>
    file !== locked && write(file, operation, details);
  syncWindowsShortcutIcon(f.options);
  assert.equal(f.errors.length, 2);
  assert.deepEqual(
    f.writes.map((entry) => entry.file),
    [good],
  );
});

test("falls back to the window icon if the persistent icon cannot be written", (t) => {
  const f = fixture(t);
  fs.writeFileSync(f.options.userData, "not a directory");
  assert.equal(syncWindowsShortcutIcon(f.options), f.options.iconPath);
  assert.equal(f.errors.length, 1);
  assert.deepEqual(f.writes, []);
});

test("checks Windows paths case-insensitively and rejects ambiguous launch commands", () => {
  const root = "C:\\Users\\Test\\AppData\\Local\\csgclaw_desktop";
  const shortcut = {
    target: root.toUpperCase() + "\\UPDATE.EXE",
    appUserModelId: windowsAppUserModelID,
  };
  assert.equal(
    isInstalledSquirrelShortcut(
      { ...shortcut, args: '--processStart "CSGClaw.exe"' },
      root,
    ),
    true,
  );
  assert.equal(
    isInstalledSquirrelShortcut(
      { ...shortcut, target: root + "\\CSGClaw.exe" },
      root,
    ),
    true,
  );
  for (const args of [
    undefined,
    "",
    "--processStart other.exe",
    "--processStart CSGClaw.exe.extra",
    "--processStart CSGClaw.exe --other",
  ]) {
    assert.equal(
      isInstalledSquirrelShortcut({ ...shortcut, args }, root),
      false,
    );
  }
});

import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import type { ShortcutDetails } from "electron";
import { DesktopPlatform } from "../shared/desktopEnvironment";
import { windowsAppUserModelID } from "./windowsTaskbar";

type ShortcutShell = {
  readShortcutLink(shortcutPath: string): ShortcutDetails;
  writeShortcutLink(
    shortcutPath: string,
    operation: "update",
    options: ShortcutDetails,
  ): boolean;
};

export type WindowsShortcutIconOptions = {
  platform: NodeJS.Platform;
  windowsStore: boolean | undefined;
  packaged: boolean;
  executablePath: string;
  appData: string;
  userData: string;
  desktop: string;
  iconPath: string;
  shell: ShortcutShell;
  onError: (error: unknown) => void;
};

export function isInstalledSquirrelShortcut(
  shortcut: ShortcutDetails,
  installRoot: string,
): boolean {
  if (shortcut.appUserModelId !== windowsAppUserModelID) return false;
  const target = path.win32.normalize(shortcut.target).toLowerCase();
  const root = path.win32.normalize(installRoot).toLowerCase();
  if (target === path.win32.join(root, "csgclaw.exe")) return true;
  return (
    target === path.win32.join(root, "update.exe") &&
    /^--processStart\s+(?:"CSGClaw\.exe"|CSGClaw\.exe)\s*$/i.test(
      (shortcut.args ?? "").trim(),
    )
  );
}

export function syncWindowsShortcutIcon(
  options: WindowsShortcutIconOptions,
): string {
  const { iconPath } = options;
  if (
    options.platform !== DesktopPlatform.Windows ||
    options.windowsStore ||
    !options.packaged
  ) {
    return iconPath;
  }
  const versionDirectory = path.dirname(options.executablePath);
  const installRoot = path.dirname(versionDirectory);
  if (
    !/^app-\d/i.test(path.basename(versionDirectory)) ||
    path.basename(options.executablePath).toLowerCase() !== "csgclaw.exe" ||
    !fs.existsSync(path.join(installRoot, "Update.exe"))
  ) {
    return iconPath;
  }

  try {
    // Pins can outlive app-<version>. Keep immutable icon resources outside it,
    // so Squirrel deleting an old release cannot leave a pin with a missing icon.
    const bytes = fs.readFileSync(iconPath);
    const digest = createHash("sha256").update(bytes).digest("hex");
    const iconDirectory = path.join(options.userData, "taskbar-icons");
    const persistentIcon = path.join(iconDirectory, `${digest}.ico`);
    fs.mkdirSync(iconDirectory, { recursive: true });
    if (!fs.existsSync(persistentIcon))
      fs.writeFileSync(persistentIcon, bytes, { flag: "wx" });

    const programs = path.join(
      options.appData,
      "Microsoft",
      "Windows",
      "Start Menu",
      "Programs",
    );
    const directories = [
      programs,
      path.join(programs, "OpenCSG"),
      options.desktop,
      path.join(
        options.appData,
        "Microsoft",
        "Internet Explorer",
        "Quick Launch",
        "User Pinned",
        "TaskBar",
      ),
    ];
    for (const directory of directories) {
      let entries: fs.Dirent[];
      try {
        entries = fs.readdirSync(directory, { withFileTypes: true });
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "ENOENT")
          options.onError(error);
        continue;
      }
      for (const entry of entries) {
        if (
          !entry.isFile() ||
          path.extname(entry.name).toLowerCase() !== ".lnk"
        )
          continue;
        const shortcutPath = path.join(directory, entry.name);
        try {
          const shortcut = options.shell.readShortcutLink(shortcutPath);
          if (
            !isInstalledSquirrelShortcut(shortcut, installRoot) ||
            (shortcut.icon === persistentIcon && shortcut.iconIndex === 0)
          )
            continue;
          // Update only presentation. Chromium preserves the other properties
          // and calls SHChangeNotify(SHCNE_UPDATEITEM) after saving the link.
          // Never create, repin, change the AUMID, or rewrite the launch target.
          // Electron's types require target even though the native "update"
          // operation explicitly allows omitted fields and preserves them.
          const iconDetails = {
            icon: persistentIcon,
            iconIndex: 0,
          } as ShortcutDetails;
          if (
            !options.shell.writeShortcutLink(
              shortcutPath,
              "update",
              iconDetails,
            )
          ) {
            throw new Error(
              `Could not update CSGClaw shortcut icon: ${shortcutPath}`,
            );
          }
        } catch (error) {
          options.onError(error);
        }
      }
    }
    return persistentIcon;
  } catch (error) {
    options.onError(error);
    return iconPath;
  }
}

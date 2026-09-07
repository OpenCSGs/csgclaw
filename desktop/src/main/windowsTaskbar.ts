import type { BrowserWindow, NativeImage } from "electron";
import { DesktopPlatform } from "../shared/desktopEnvironment";

export const windowsAppUserModelID = "com.squirrel.csgclaw_desktop.CSGClaw";

export type WindowsTaskbarAppDetails = {
  appId: string;
  appIconPath: string;
  appIconIndex: number;
};

export function windowsTaskbarAppDetails(
  platform: NodeJS.Platform,
  windowsStore: boolean | undefined,
  iconPath: string,
): WindowsTaskbarAppDetails | undefined {
  if (
    platform !== DesktopPlatform.Windows ||
    windowsStore === true ||
    !iconPath
  ) {
    return undefined;
  }
  return {
    appId: windowsAppUserModelID,
    appIconPath: iconPath,
    appIconIndex: 0,
  };
}

type TaskbarWindow = Pick<
  BrowserWindow,
  "isDestroyed" | "isVisible" | "setIcon" | "setAppDetails" | "setSkipTaskbar"
>;

type ThemeIcon = {
  image: NativeImage;
  path: string;
  sourcePath: string;
};

export class WindowsTaskbarIcon {
  private selected: ThemeIcon | null = null;
  private readonly applied = new WeakMap<TaskbarWindow, ThemeIcon>();

  constructor(
    private readonly platform: NodeJS.Platform,
    private readonly windowsStore: boolean | undefined,
    private readonly syncShortcutIcon: (iconPath: string) => string = (
      iconPath,
    ) => iconPath,
  ) {}

  get image(): NativeImage | undefined {
    return this.selected?.image;
  }

  select(image: NativeImage, iconPath: string): void {
    if (
      this.platform !== DesktopPlatform.Windows ||
      !iconPath ||
      image.isEmpty() ||
      this.selected?.sourcePath === iconPath
    ) {
      return;
    }
    let path = iconPath;
    if (!this.windowsStore) {
      try {
        path = this.syncShortcutIcon(iconPath);
      } catch {
        path = iconPath;
      }
    }
    this.selected = {
      image,
      path,
      sourcePath: iconPath,
    };
  }

  apply(window: TaskbarWindow, force = false): void {
    this.applySelected(window, force, false);
  }

  refresh(window: TaskbarWindow): void {
    this.applySelected(window, true, true);
  }

  private applySelected(
    window: TaskbarWindow,
    force: boolean,
    recreateButton: boolean,
  ): void {
    const icon = this.selected;
    if (
      !icon ||
      window.isDestroyed() ||
      (!force && this.applied.get(window) === icon)
    ) {
      return;
    }
    const previous = this.applied.get(window);
    const refreshButton =
      !this.windowsStore &&
      previous !== undefined &&
      (previous !== icon || recreateButton) &&
      window.isVisible();
    const details = windowsTaskbarAppDetails(
      this.platform,
      this.windowsStore,
      icon.path,
    );
    // Recreate only an existing visible button after the matching shortcut has
    // been updated, so Explorer can reread its full icon under the same AUMID.
    if (refreshButton) window.setSkipTaskbar(true);
    try {
      if (details) window.setAppDetails(details);
      window.setIcon(icon.image);
    } finally {
      if (refreshButton && !window.isDestroyed()) window.setSkipTaskbar(false);
    }
    this.applied.set(window, icon);
  }
}

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

const waitForWindowsTaskbarRemoval = (): Promise<void> =>
  new Promise((resolve) => setTimeout(resolve, 75));

export class WindowsTaskbarIcon {
  private selected: ThemeIcon | null = null;
  private readonly applied = new WeakMap<TaskbarWindow, ThemeIcon>();
  private refreshRevision = 0;

  constructor(
    private readonly platform: NodeJS.Platform,
    private readonly windowsStore: boolean | undefined,
    private readonly syncShortcutIcon: (iconPath: string) => string = (
      iconPath,
    ) => iconPath,
    private readonly waitForTaskbarRemoval: () => Promise<void> =
      waitForWindowsTaskbarRemoval,
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
    const icon = this.selected;
    if (
      !icon ||
      window.isDestroyed() ||
      (!force && this.applied.get(window) === icon)
    ) {
      return;
    }
    const details = windowsTaskbarAppDetails(
      this.platform,
      this.windowsStore,
      icon.path,
    );
    if (details) window.setAppDetails(details);
    window.setIcon(icon.image);
    this.applied.set(window, icon);
  }

  async refresh(window: TaskbarWindow): Promise<void> {
    if (!this.selected || window.isDestroyed()) {
      return;
    }
    if (this.windowsStore || !window.isVisible()) {
      this.apply(window, true);
      return;
    }

    const revision = ++this.refreshRevision;
    window.setSkipTaskbar(true);
    try {
      // Give Explorer a separate message-loop window to remove the old button.
      // Applying the latest selected icon only after that gap avoids Windows
      // coalescing an immediate true -> false transition during rapid changes.
      await this.waitForTaskbarRemoval();
      if (revision !== this.refreshRevision || window.isDestroyed()) {
        return;
      }
      this.apply(window, true);
    } finally {
      // A newer refresh owns restoring the button. An older completion must not
      // make the taskbar visible early with an intermediate theme.
      if (revision === this.refreshRevision && !window.isDestroyed()) {
        window.setSkipTaskbar(false);
      }
    }
  }
}

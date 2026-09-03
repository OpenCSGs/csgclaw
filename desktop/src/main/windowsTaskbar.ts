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

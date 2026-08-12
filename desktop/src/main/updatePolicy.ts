import { DesktopPlatform } from "../shared/desktopEnvironment";
import { compareDesktopReleaseVersions } from "../shared/releaseVersion";

export function usesMicrosoftStoreUpdates(
  platform: NodeJS.Platform,
  windowsStore: boolean | undefined,
): boolean {
  return platform === DesktopPlatform.Windows && windowsStore === true;
}

export function shouldInstallDesktopVersion(
  currentVersion: string,
  targetVersion: string,
  channelSwitch: boolean,
): boolean {
  const comparison = compareDesktopReleaseVersions(
    currentVersion,
    targetVersion,
  );
  return channelSwitch ? comparison !== 0 : comparison < 0;
}

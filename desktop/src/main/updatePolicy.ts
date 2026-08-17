import { DesktopPlatform } from "../shared/desktopEnvironment";
import { compareDesktopReleaseVersions } from "../shared/releaseVersion";

export const SQUIRREL_FIRST_RUN_UPDATE_DELAY_MS = 60_000;

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

export function squirrelFirstRunUpdateDelay(
  platform: NodeJS.Platform,
  argv: readonly string[],
): number {
  if (platform !== DesktopPlatform.Windows) {
    return 0;
  }
  return argv.some(
    (argument) => argument.toLowerCase() === "--squirrel-firstrun",
  )
    ? SQUIRREL_FIRST_RUN_UPDATE_DELAY_MS
    : 0;
}

export function isSquirrelUpdateLockError(message: string): boolean {
  return message.toLowerCase().includes("couldn't acquire lock");
}

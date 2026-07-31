import { DesktopPlatform } from "../shared/desktopEnvironment";

export function usesMicrosoftStoreUpdates(
  platform: NodeJS.Platform,
  windowsStore: boolean | undefined,
): boolean {
  return platform === DesktopPlatform.Windows && windowsStore === true;
}

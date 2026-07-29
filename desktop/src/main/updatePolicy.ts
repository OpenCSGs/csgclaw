export function usesMicrosoftStoreUpdates(
  platform: NodeJS.Platform,
  windowsStore: boolean | undefined,
): boolean {
  return platform === "win32" && windowsStore === true;
}

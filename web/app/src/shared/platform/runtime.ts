import { getDesktopBridge } from "./desktopBridge";

export type PlatformRuntime = "browser" | "desktop";

export function platformRuntime(): PlatformRuntime {
  return getDesktopBridge() ? "desktop" : "browser";
}

export function isDesktopRuntime(): boolean {
  return platformRuntime() === "desktop";
}

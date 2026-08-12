export type DesktopPlatform = "darwin" | "win32" | "linux";
export type DesktopOAuthPurpose = "opencsg-auth" | "github-connector";
export type DesktopThemeSource = "system" | "light" | "dark";
export type DesktopUpdateChannel = "release" | "beta";

export type DesktopRuntimeInfo = {
  platform: DesktopPlatform;
  arch: string;
  appVersion: string;
  backendVersion: string;
};

export type DesktopUpdateStatus = {
  state: "idle" | "checking" | "available" | "downloading" | "not-available" | "downloaded" | "error" | "unsupported";
  channel: DesktopUpdateChannel;
  currentVersion: string;
  availableVersion?: string;
  message?: string;
};

export type DesktopBridge = {
  getRuntimeInfo(): Promise<DesktopRuntimeInfo>;
  openOAuth(input: { purpose: DesktopOAuthPurpose; url: string }): Promise<{ opened: boolean }>;
  checkForUpdates(): Promise<DesktopUpdateStatus>;
  installDownloadedUpdate(): Promise<void>;
  restartSidecar(): Promise<void>;
  setUpdateChannel(channel: DesktopUpdateChannel): Promise<DesktopUpdateStatus>;
  setThemeSource(theme: DesktopThemeSource): Promise<void>;
  onUpdateStatus(listener: (status: DesktopUpdateStatus) => void): () => void;
};

declare global {
  interface Window {
    csgclawDesktop?: DesktopBridge;
  }
}

export function getDesktopBridge(): DesktopBridge | null {
  if (typeof window === "undefined") {
    return null;
  }
  const bridge = window.csgclawDesktop;
  return bridge &&
    typeof bridge.getRuntimeInfo === "function" &&
    typeof bridge.openOAuth === "function" &&
    typeof bridge.checkForUpdates === "function" &&
    typeof bridge.installDownloadedUpdate === "function" &&
    typeof bridge.restartSidecar === "function" &&
    typeof bridge.setUpdateChannel === "function" &&
    typeof bridge.setThemeSource === "function" &&
    typeof bridge.onUpdateStatus === "function"
    ? bridge
    : null;
}

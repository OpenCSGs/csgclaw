export type DesktopPlatform = "darwin" | "win32" | "linux";
export type DesktopOAuthPurpose = "opencsg-auth" | "github-connector";
export type DesktopThemeSource = "system" | "light" | "dark";

export type DesktopRuntimeInfo = {
  platform: DesktopPlatform;
  arch: string;
  appVersion: string;
  backendVersion: string;
};

export type DesktopUpdateStatus = {
  state: "idle" | "checking" | "available" | "not-available" | "downloaded" | "error" | "unsupported";
  currentVersion: string;
  availableVersion?: string;
  message?: string;
};

export type DesktopBridge = {
  getRuntimeInfo(): Promise<DesktopRuntimeInfo>;
  openOAuth(input: { purpose: DesktopOAuthPurpose; url: string }): Promise<{ opened: boolean }>;
  checkForUpdates(): Promise<void>;
  installDownloadedUpdate(): Promise<void>;
  restartSidecar(): Promise<void>;
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
    typeof bridge.setThemeSource === "function" &&
    typeof bridge.onUpdateStatus === "function"
    ? bridge
    : null;
}

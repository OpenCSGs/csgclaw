export type BrowserBounds = { x: number; y: number; width: number; height: number };
export type DesktopBrowserRequest = { id: string } & (
  | { action: "sync"; url: string; visible: boolean; bounds: BrowserBounds }
  | { action: "navigate"; url: string }
  | { action: "reload" | "back" | "forward" | "close" }
);
export type DesktopBrowserState = {
  id: string;
  url: string;
  loading: boolean;
  canGoBack: boolean;
  canGoForward: boolean;
  error?: string;
  popupURL?: string;
  shortcut?: "escape";
};

export type DesktopPlatform = "darwin" | "win32" | "linux";
export type DesktopOAuthPurpose = "opencsg-auth" | "github-connector" | "gitlab-connector";
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

export type DesktopWindowState = { focused: boolean; fullscreen: boolean };

export type DesktopBridge = {
  getWindowState(): Promise<DesktopWindowState>;
  onWindowState(listener: (state: DesktopWindowState) => void): () => void;
  browserCommand(input: DesktopBrowserRequest): Promise<DesktopBrowserState | null>;
  onBrowserState(listener: (state: DesktopBrowserState) => void): () => void;
  getRuntimeInfo(): Promise<DesktopRuntimeInfo>;
  openOAuth(input: { purpose: DesktopOAuthPurpose; url: string }): Promise<{ opened: boolean }>;
  checkForUpdates(): Promise<DesktopUpdateStatus>;
  installDownloadedUpdate(): Promise<void>;
  restartSidecar(): Promise<void>;
  showWindow(): Promise<void>;
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
    typeof bridge.getWindowState === "function" &&
    typeof bridge.onWindowState === "function" &&
    typeof bridge.browserCommand === "function" &&
    typeof bridge.onBrowserState === "function" &&
    typeof bridge.getRuntimeInfo === "function" &&
    typeof bridge.openOAuth === "function" &&
    typeof bridge.checkForUpdates === "function" &&
    typeof bridge.installDownloadedUpdate === "function" &&
    typeof bridge.restartSidecar === "function" &&
    typeof bridge.showWindow === "function" &&
    typeof bridge.setUpdateChannel === "function" &&
    typeof bridge.setThemeSource === "function" &&
    typeof bridge.onUpdateStatus === "function"
    ? bridge
    : null;
}

import { DesktopPlatform } from "./desktopEnvironment";

export { DesktopPlatform } from "./desktopEnvironment";

export const DesktopIPC = {
  checkForUpdates: "csgclaw:desktop:check-for-updates",
  getRuntimeInfo: "csgclaw:desktop:get-runtime-info",
  installDownloadedUpdate: "csgclaw:desktop:install-downloaded-update",
  openOAuth: "csgclaw:desktop:open-oauth",
  restartSidecar: "csgclaw:desktop:restart-sidecar",
  setThemeSource: "csgclaw:desktop:set-theme-source",
  setUpdateChannel: "csgclaw:desktop:set-update-channel",
  updateStatus: "csgclaw:desktop:update-status",
} as const;

export type OAuthPurpose = "opencsg-auth" | "github-connector";
export type DesktopThemeSource = "system" | "light" | "dark";
export type DesktopUpdateChannel = "release" | "beta";

export type DesktopRuntimeInfo = {
  platform: DesktopPlatform;
  arch: string;
  appVersion: string;
  backendVersion: string;
};

export type DesktopOAuthInput = {
  purpose: OAuthPurpose;
  url: string;
};

export type DesktopUpdateState =
  | "idle"
  | "checking"
  | "available"
  | "downloading"
  | "not-available"
  | "downloaded"
  | "error"
  | "unsupported";

export type DesktopUpdateStatus = {
  state: DesktopUpdateState;
  channel: DesktopUpdateChannel;
  currentVersion: string;
  availableVersion?: string;
  message?: string;
};

export type DesktopBridge = {
  getRuntimeInfo(): Promise<DesktopRuntimeInfo>;
  openOAuth(input: DesktopOAuthInput): Promise<{ opened: boolean }>;
  checkForUpdates(): Promise<DesktopUpdateStatus>;
  installDownloadedUpdate(): Promise<void>;
  restartSidecar(): Promise<void>;
  setThemeSource(theme: DesktopThemeSource): Promise<void>;
  setUpdateChannel(channel: DesktopUpdateChannel): Promise<DesktopUpdateStatus>;
  onUpdateStatus(listener: (status: DesktopUpdateStatus) => void): () => void;
};

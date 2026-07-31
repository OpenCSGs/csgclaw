export const DesktopIPC = {
  checkForUpdates: "csgclaw:desktop:check-for-updates",
  getRuntimeInfo: "csgclaw:desktop:get-runtime-info",
  installDownloadedUpdate: "csgclaw:desktop:install-downloaded-update",
  openOAuth: "csgclaw:desktop:open-oauth",
  restartSidecar: "csgclaw:desktop:restart-sidecar",
  updateStatus: "csgclaw:desktop:update-status",
} as const;

export type DesktopPlatform = "darwin" | "win32" | "linux";
export type OAuthPurpose = "opencsg-auth" | "github-connector";

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
  | "not-available"
  | "downloaded"
  | "error"
  | "unsupported";

export type DesktopUpdateStatus = {
  state: DesktopUpdateState;
  currentVersion: string;
  availableVersion?: string;
  message?: string;
};

export type DesktopBridge = {
  getRuntimeInfo(): Promise<DesktopRuntimeInfo>;
  openOAuth(input: DesktopOAuthInput): Promise<{ opened: boolean }>;
  checkForUpdates(): Promise<void>;
  installDownloadedUpdate(): Promise<void>;
  restartSidecar(): Promise<void>;
  onUpdateStatus(listener: (status: DesktopUpdateStatus) => void): () => void;
};

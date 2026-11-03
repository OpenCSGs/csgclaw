import type {
  DesktopBrowserRequest,
  DesktopBrowserState,
} from "./desktopBrowser.types";
import { DesktopPlatform } from "./desktopEnvironment";

export { DesktopPlatform } from "./desktopEnvironment";

export const DesktopIPC = {
  getWindowState: "csgclaw:desktop:get-window-state",
  windowState: "csgclaw:desktop:window-state",
  browserCommand: "csgclaw:desktop:browser-command",
  browserState: "csgclaw:desktop:browser-state",
  checkForUpdates: "csgclaw:desktop:check-for-updates",
  getRuntimeInfo: "csgclaw:desktop:get-runtime-info",
  installDownloadedUpdate: "csgclaw:desktop:install-downloaded-update",
  openOAuth: "csgclaw:desktop:open-oauth",
  restartSidecar: "csgclaw:desktop:restart-sidecar",
  showWindow: "csgclaw:desktop:show-window",
  showTurnNotification: "csgclaw:desktop:show-turn-notification",
  turnNotificationClick: "csgclaw:desktop:turn-notification-click",
  setThemeSource: "csgclaw:desktop:set-theme-source",
  setWindowChromeAppearance: "csgclaw:desktop:set-window-chrome-appearance",
  setUpdateChannel: "csgclaw:desktop:set-update-channel",
  updateStatus: "csgclaw:desktop:update-status",
} as const;

export type OAuthPurpose =
  | "opencsg-auth"
  | "github-connector"
  | "gitlab-connector";
export type DesktopThemeSource = "system" | "light" | "dark";
export type DesktopWindowChromeAppearance = "default" | "onboarding";
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
  /** Installed channel derived from currentVersion, never a pending switch target. */
  channel: DesktopUpdateChannel;
  currentVersion: string;
  availableVersion?: string;
  message?: string;
};

export type DesktopTurnNotification = {
  silent: boolean;
  id: string;
  title: string;
  body: string;
  roomID: string;
};

export type DesktopWindowState = { focused: boolean; fullscreen: boolean };

export type DesktopBridge = {
  getWindowState(): Promise<DesktopWindowState>;
  onWindowState(listener: (state: DesktopWindowState) => void): () => void;
  browserCommand(
    input: DesktopBrowserRequest,
  ): Promise<DesktopBrowserState | null>;
  onBrowserState(listener: (state: DesktopBrowserState) => void): () => void;
  getRuntimeInfo(): Promise<DesktopRuntimeInfo>;
  openOAuth(input: DesktopOAuthInput): Promise<{ opened: boolean }>;
  checkForUpdates(): Promise<DesktopUpdateStatus>;
  installDownloadedUpdate(): Promise<void>;
  restartSidecar(): Promise<void>;
  showWindow(): Promise<void>;
  showTurnNotification(input: DesktopTurnNotification): Promise<void>;
  onTurnNotificationClick(listener: (roomID: string) => void): () => void;
  setThemeSource(theme: DesktopThemeSource): Promise<void>;
  setWindowChromeAppearance?(
    appearance: DesktopWindowChromeAppearance,
  ): Promise<void>;
  setUpdateChannel(channel: DesktopUpdateChannel): Promise<DesktopUpdateStatus>;
  onUpdateStatus(listener: (status: DesktopUpdateStatus) => void): () => void;
};

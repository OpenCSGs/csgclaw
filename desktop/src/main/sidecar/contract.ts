export const DESKTOP_PROTOCOL_VERSION = 1;

export const DesktopMessageType = {
  bootstrap: "csgclaw.desktop.bootstrap",
  ready: "csgclaw.desktop.ready",
  shutdown: "csgclaw.desktop.shutdown",
} as const;

export type DesktopBootstrapMessage = {
  type: typeof DesktopMessageType.bootstrap;
  protocol_version: typeof DESKTOP_PROTOCOL_VERSION;
  instance_id: string;
  session_token: string;
};

export type DesktopReadyMessage = {
  type: typeof DesktopMessageType.ready;
  protocol_version: typeof DESKTOP_PROTOCOL_VERSION;
  instance_id: string;
  pid: number;
  base_url: string;
  version: string;
  distribution: "electron";
};

export type DesktopShutdownMessage = {
  type: typeof DesktopMessageType.shutdown;
  reason: string;
};

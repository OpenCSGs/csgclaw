import { applyUpgradeRequest, fetchUpgradeStatus, setUpgradeChannelRequest } from "@/api/upgrade";
import { normalizeUpgradeStatus, type UpgradeChannel, type UpgradeStatus } from "@/models/upgradeStatus";
import { getDesktopBridge, type DesktopUpdateStatus } from "./desktopBridge";

export type UpgradeApplyMode = "browser-daemon" | "desktop-app";
export type UpgradeStatusListener = (status: UpgradeStatus) => void;

let cachedDesktopStatus: UpgradeStatus | null = null;
let cleanupDesktopSubscription: (() => void) | null = null;
const desktopListeners = new Set<UpgradeStatusListener>();

export async function fetchPlatformUpgradeStatus(): Promise<UpgradeStatus> {
  const bridge = getDesktopBridge();
  if (!bridge) {
    return normalizeUpgradeStatus(await fetchUpgradeStatus()) ?? emptyUpgradeStatus();
  }
  ensureDesktopSubscription();
  cachedDesktopStatus = desktopUpdateStatus(await bridge.checkForUpdates());
  return cachedDesktopStatus;
}

export async function applyPlatformUpgrade(): Promise<UpgradeApplyMode> {
  const bridge = getDesktopBridge();
  if (!bridge) {
    await applyUpgradeRequest();
    return "browser-daemon";
  }
  await bridge.installDownloadedUpdate();
  return "desktop-app";
}

export async function setPlatformUpgradeChannel(channel: UpgradeChannel): Promise<UpgradeStatus> {
  const bridge = getDesktopBridge();
  if (!bridge) {
    return normalizeUpgradeStatus(await setUpgradeChannelRequest(channel)) ?? emptyUpgradeStatus();
  }
  ensureDesktopSubscription();
  cachedDesktopStatus = desktopUpdateStatus(await bridge.setUpdateChannel(channel));
  return cachedDesktopStatus;
}

export function subscribePlatformUpgradeStatus(listener: UpgradeStatusListener): () => void {
  if (!getDesktopBridge()) {
    return () => undefined;
  }
  ensureDesktopSubscription();
  desktopListeners.add(listener);
  if (cachedDesktopStatus) {
    listener(cachedDesktopStatus);
  }
  return () => {
    desktopListeners.delete(listener);
  };
}

function ensureDesktopSubscription(): void {
  const bridge = getDesktopBridge();
  if (!bridge || cleanupDesktopSubscription) {
    return;
  }
  cleanupDesktopSubscription = bridge.onUpdateStatus((status) => {
    cachedDesktopStatus = desktopUpdateStatus(status);
    for (const listener of desktopListeners) {
      listener(cachedDesktopStatus);
    }
  });
}

export function desktopUpdateStatus(status: DesktopUpdateStatus): UpgradeStatus {
  const downloaded = status.state === "downloaded";
  const downloading = status.state === "downloading";
  const available = status.state === "available";
  const error = status.state === "error";
  const unsupported = status.state === "unsupported";
  return {
    auto_upgrade_supported: !unsupported,
    auto_upgrade_unsupported_reason: unsupported ? "desktop_update_unsupported" : "",
    channel: status.channel,
    checking: status.state === "checking",
    current_version: status.currentVersion,
    downloaded,
    last_checked_at: new Date().toISOString(),
    last_error: error ? status.message || "Desktop update failed." : "",
    last_error_kind: error ? "desktop_update" : "",
    last_error_log_path: "",
    latest_version: status.availableVersion || "",
    manual_restart_required: false,
    update_available: available || downloading || downloaded,
    upgrading: downloading,
  };
}

function emptyUpgradeStatus(currentVersion = "", channel: UpgradeChannel = "release"): UpgradeStatus {
  return {
    auto_upgrade_supported: true,
    auto_upgrade_unsupported_reason: "",
    channel,
    checking: false,
    current_version: currentVersion,
    downloaded: false,
    last_checked_at: "",
    last_error: "",
    last_error_kind: "",
    last_error_log_path: "",
    latest_version: "",
    manual_restart_required: false,
    update_available: false,
    upgrading: false,
  };
}

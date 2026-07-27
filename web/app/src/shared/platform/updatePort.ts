import { applyUpgradeRequest, fetchUpgradeStatus } from "@/api/upgrade";
import { normalizeUpgradeStatus, type UpgradeStatus } from "@/models/upgradeStatus";
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
  const runtime = await bridge.getRuntimeInfo();
  if (!cachedDesktopStatus) {
    cachedDesktopStatus = emptyUpgradeStatus(runtime.appVersion);
  }
  await bridge.checkForUpdates();
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

function desktopUpdateStatus(status: DesktopUpdateStatus): UpgradeStatus {
  const downloaded = status.state === "downloaded";
  const error = status.state === "error";
  const unsupported = status.state === "unsupported";
  return {
    auto_upgrade_supported: !unsupported,
    auto_upgrade_unsupported_reason: unsupported ? "desktop_update_unsupported" : "",
    checking: status.state === "checking" || status.state === "available",
    current_version: status.currentVersion,
    last_checked_at: new Date().toISOString(),
    last_error: error ? status.message || "Desktop update failed." : "",
    last_error_kind: error ? "desktop_update" : "",
    last_error_log_path: "",
    latest_version: status.availableVersion || "",
    manual_restart_required: false,
    update_available: downloaded,
    upgrading: false,
  };
}

function emptyUpgradeStatus(currentVersion = ""): UpgradeStatus {
  return {
    auto_upgrade_supported: true,
    auto_upgrade_unsupported_reason: "",
    checking: false,
    current_version: currentVersion,
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

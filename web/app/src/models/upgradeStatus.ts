// @ts-nocheck
// API returns Version from git describe (e.g. "v0.2.1-5-gabc-dirty") or "dev"; avoid "vv" in the UI.
export function formatSidebarVersionLabel(version) {
  const raw = typeof version === "string" ? version.trim() : "";
  if (!raw) {
    return "csgclaw dev";
  }
  return raw.startsWith("v") ? `csgclaw ${raw}` : `csgclaw v${raw}`;
}

export function normalizeUpgradeStatus(status) {
  if (!status || typeof status !== "object") {
    return null;
  }
  return {
    current_version: typeof status.current_version === "string" ? status.current_version : "",
    latest_version: typeof status.latest_version === "string" ? status.latest_version : "",
    update_available: Boolean(status.update_available),
    checking: Boolean(status.checking),
    upgrading: Boolean(status.upgrading),
    last_checked_at: status.last_checked_at || "",
    last_error: typeof status.last_error === "string" ? status.last_error : "",
  };
}

export function upgradeStatusLabel(phase, t) {
  switch (phase) {
    case "starting":
      return t("upgradeStatusStarting");
    case "restarting":
      return t("upgradeStatusRestarting");
    case "done":
      return t("upgradeStatusDone");
    case "error":
      return t("upgradeStatusError");
    default:
      return t("upgradeStatusReady");
  }
}

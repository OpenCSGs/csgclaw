// API returns Version from git describe (e.g. "v0.2.1-5-gabc-dirty") or "dev"; avoid "vv" in the UI.
import type { TranslateFn } from "@/models/conversations";

export type UpgradeStatus = {
  checking: boolean;
  current_version: string;
  last_checked_at: unknown;
  last_error: string;
  latest_version: string;
  update_available: boolean;
  upgrading: boolean;
};

export type UpgradePhase = "idle" | "starting" | "restarting" | "done" | "error";

export function formatSidebarVersionLabel(version: unknown): string {
  const raw = typeof version === "string" ? version.trim() : "";
  if (!raw) {
    return "csgclaw dev";
  }
  return raw.startsWith("v") ? `csgclaw ${raw}` : `csgclaw v${raw}`;
}

export function normalizeUpgradeStatus(status: unknown): UpgradeStatus | null {
  if (!status || typeof status !== "object") {
    return null;
  }
  const source = status as Partial<Record<keyof UpgradeStatus, unknown>>;
  return {
    current_version: typeof source.current_version === "string" ? source.current_version : "",
    latest_version: typeof source.latest_version === "string" ? source.latest_version : "",
    update_available: Boolean(source.update_available),
    checking: Boolean(source.checking),
    upgrading: Boolean(source.upgrading),
    last_checked_at: source.last_checked_at || "",
    last_error: typeof source.last_error === "string" ? source.last_error : "",
  };
}

export function upgradeStatusLabel(phase: UpgradePhase, t: TranslateFn): string {
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

export function hasUpgradeAttention(
  status: UpgradeStatus | null | undefined,
  phase: UpgradePhase,
  busy = false,
): boolean {
  return Boolean(
    status?.update_available ||
    status?.upgrading ||
    status?.last_error ||
    busy ||
    phase === "done" ||
    phase === "error",
  );
}

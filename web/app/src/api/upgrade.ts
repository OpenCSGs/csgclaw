import { get, post } from "@/api/client";
import type { UpgradeStatus } from "@/models/upgradeStatus";
import { ApiEndpoints } from "@/shared/constants/api";

export function fetchUpgradeStatus(): Promise<UpgradeStatus> {
  return get(ApiEndpoints.upgradeStatus);
}

export function applyUpgradeRequest(): Promise<void> {
  return post(ApiEndpoints.upgradeApply);
}

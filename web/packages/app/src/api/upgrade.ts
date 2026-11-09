import { get, post, put } from "@/api/client";
import type { UpgradeChannel, UpgradeStatus } from "@/models/upgradeStatus";
import { ApiEndpoints } from "@/shared/constants/api";

export function fetchUpgradeStatus(): Promise<UpgradeStatus> {
  return get(ApiEndpoints.upgradeStatus);
}

export function applyUpgradeRequest(): Promise<void> {
  return post(ApiEndpoints.upgradeApply);
}

export function setUpgradeChannelRequest(channel: UpgradeChannel): Promise<UpgradeStatus> {
  return put(ApiEndpoints.upgradeChannel, { channel });
}

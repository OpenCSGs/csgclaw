// @ts-nocheck
import { UPGRADE_APPLY_ENDPOINT, UPGRADE_STATUS_ENDPOINT } from "@/bootstrap/constants";
import { get, post } from "@/api/client";

export function fetchUpgradeStatus() {
  return get(UPGRADE_STATUS_ENDPOINT);
}

export function applyUpgradeRequest() {
  return post(UPGRADE_APPLY_ENDPOINT);
}

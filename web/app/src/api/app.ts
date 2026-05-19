// @ts-nocheck
import { VERSION_ENDPOINT } from "@/bootstrap/constants";
import { get } from "@/api/client";

export function fetchBootstrap() {
  return get("api/v1/bootstrap");
}

export function fetchBootstrapConfig() {
  return get("api/v1/config/bootstrap");
}

export function fetchVersion(options = {}) {
  const path = options.cacheBust ? `${VERSION_ENDPOINT}?_=${Date.now()}` : VERSION_ENDPOINT;
  return get(path, options.cacheBust ? { cache: "no-store" } : undefined);
}

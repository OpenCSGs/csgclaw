import { get } from "@/api/client";
import { ApiEndpoints } from "@/shared/constants/api";
import type { RuntimeBootstrapConfig } from "@/models/agents";
import type { IMData } from "@/models/conversations";

export type FetchVersionOptions = {
  cacheBust?: boolean;
};

export type VersionResponse = {
  version?: string | null;
};

export function fetchBootstrap(): Promise<IMData> {
  return get("api/v1/bootstrap");
}

export function fetchBootstrapConfig(): Promise<RuntimeBootstrapConfig> {
  return get("api/v1/config/bootstrap");
}

export function fetchRuntimeImages(): Promise<string[]> {
  return get("api/v1/agents/image-candidates");
}

export function fetchVersion(options: FetchVersionOptions = {}): Promise<VersionResponse> {
  const path = options.cacheBust ? `${ApiEndpoints.version}?_=${Date.now()}` : ApiEndpoints.version;
  return get(path, options.cacheBust ? { cache: "no-store" } : undefined);
}

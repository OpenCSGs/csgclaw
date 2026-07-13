import { get, post } from "@/api/client";
import { ApiEndpoints } from "@/shared/constants/api";

export function fetchAgentRuntimes(): Promise<unknown> {
  return get(ApiEndpoints.agentRuntimes, { cache: "no-store" });
}

export function installAgentRuntimeRequest(runtimeName: string): Promise<unknown> {
  const name = encodeURIComponent(String(runtimeName ?? "").trim());
  return post(`${ApiEndpoints.agentRuntimes}/${name}/install`);
}

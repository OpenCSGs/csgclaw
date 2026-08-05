import { get } from "@/api/client";
import { ApiEndpoints } from "@/shared/constants/api";

export function fetchAgentRuntimes(): Promise<unknown> {
  return get(ApiEndpoints.agentRuntimes, { cache: "no-store" });
}

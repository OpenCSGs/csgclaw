import { post } from "@/api/client";

export function fetchDescriptionAccess(signal: AbortSignal): Promise<{ allowed: boolean }> {
  return post("api/v1/description-access", {}, { signal });
}

import { get, post } from "@/api/client";
import type { AuthEnvironmentLoginPayload } from "@/models/authEnvironment";
import { ApiEndpoints } from "@/shared/constants/api";

export function fetchAuthStatus(): Promise<unknown> {
  return get(ApiEndpoints.authStatus);
}

export function beginAuthLogin(returnURL = "", environment?: AuthEnvironmentLoginPayload): Promise<unknown> {
  return post(ApiEndpoints.authLogin, { return_url: returnURL, ...environment });
}

export function logoutAuth(): Promise<unknown> {
  return post(ApiEndpoints.authLogout);
}

export type AuthLoginSite = {
  id: string;
  url: string;
  icon: string;
  locales: Record<"zh" | "en", { title: string; description: string }>;
};

export async function fetchAuthLoginSites(signal: AbortSignal): Promise<AuthLoginSite[]> {
  const catalog = await get<{ sites: AuthLoginSite[] }>(ApiEndpoints.authLoginSites, { signal });
  return catalog.sites;
}

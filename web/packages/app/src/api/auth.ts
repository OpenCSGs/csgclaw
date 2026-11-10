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

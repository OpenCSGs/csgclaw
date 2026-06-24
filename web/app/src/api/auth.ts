import { get, post } from "@/api/client";
import { ApiEndpoints } from "@/shared/constants/api";

export function fetchAuthStatus(): Promise<unknown> {
  return get(ApiEndpoints.authStatus);
}

export function beginAuthLogin(returnURL = ""): Promise<unknown> {
  return post(ApiEndpoints.authLogin, { return_url: returnURL });
}

export function logoutAuth(): Promise<unknown> {
  return post(ApiEndpoints.authLogout);
}

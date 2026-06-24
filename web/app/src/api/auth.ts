import { get, post } from "@/api/client";
import { ApiEndpoints } from "@/shared/constants/api";

export function fetchAuthStatus(): Promise<unknown> {
  return get(ApiEndpoints.authStatus);
}

export type AuthLoginOptions = {
  suppressReturnURL?: boolean;
};

export function beginAuthLogin(returnURL = "", options: AuthLoginOptions = {}): Promise<unknown> {
  return post(ApiEndpoints.authLogin, {
    return_url: returnURL,
    suppress_return_url: options.suppressReturnURL || undefined,
  });
}

export function logoutAuth(): Promise<unknown> {
  return post(ApiEndpoints.authLogout);
}

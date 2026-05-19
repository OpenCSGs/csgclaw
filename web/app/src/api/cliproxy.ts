// @ts-nocheck
import { get, post } from "@/api/client";

export function fetchCLIProxyAuthStatus(provider) {
  return get(`api/v1/cliproxy/auth/status?provider=${encodeURIComponent(provider)}`);
}

export function loginCLIProxyProviderRequest(provider) {
  return post("api/v1/cliproxy/auth/login", { provider });
}

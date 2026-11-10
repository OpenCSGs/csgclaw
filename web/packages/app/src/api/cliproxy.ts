import { get, post } from "@/api/client";

export type CLIProxyAuthStatus = {
  authenticated?: boolean;
  login_required?: boolean;
  message?: string;
  provider?: string;
};

export function fetchCLIProxyAuthStatus(provider: string): Promise<CLIProxyAuthStatus> {
  return get(`api/v1/cliproxy/auth/status?provider=${encodeURIComponent(provider)}`);
}

export function loginCLIProxyProviderRequest(provider: string): Promise<CLIProxyAuthStatus> {
  return post("api/v1/cliproxy/auth/login", { provider });
}

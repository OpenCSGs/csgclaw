const SYSTEM_PROXY_PROBE_URL = "https://api.openai.com/";
export const CLIPROXY_SYSTEM_PROXY_ENV = "CSGCLAW_CLIPROXY_SYSTEM_PROXY_URL";

export type SystemProxyResolver = (url: string) => Promise<string>;

export async function resolveSystemCLIProxyEnvironment(
  resolveProxy: SystemProxyResolver,
): Promise<NodeJS.ProcessEnv> {
  try {
    const proxyURL = firstProxyURL(await resolveProxy(SYSTEM_PROXY_PROBE_URL));
    if (!proxyURL) {
      return {};
    }
    return { [CLIPROXY_SYSTEM_PROXY_ENV]: proxyURL };
  } catch {
    return {};
  }
}

export function firstProxyURL(rules: string): string | undefined {
  for (const rawRule of rules.split(";")) {
    const rule = rawRule.trim();
    if (!rule || rule.toUpperCase() === "DIRECT") {
      continue;
    }
    const match = /^(PROXY|HTTP|HTTPS|SOCKS|SOCKS4|SOCKS5)\s+(.+)$/i.exec(rule);
    if (!match) {
      continue;
    }
    const [, rawType, rawAddress] = match;
    if (!rawType || !rawAddress) {
      continue;
    }
    const scheme = proxyScheme(rawType.toUpperCase());
    try {
      const proxyURL = new URL(`${scheme}://${rawAddress.trim()}`);
      if (!proxyURL.hostname || !proxyURL.port || proxyURL.username || proxyURL.password) {
        continue;
      }
      return proxyURL.toString().replace(/\/$/, "");
    } catch {
      continue;
    }
  }
  return undefined;
}

function proxyScheme(type: string): string {
  switch (type) {
    case "HTTPS":
      return "https";
    case "SOCKS4":
      return "socks4";
    case "SOCKS":
    case "SOCKS5":
      return "socks5";
    default:
      return "http";
  }
}

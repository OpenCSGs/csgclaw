const SYSTEM_PROXY_URLS = {
  HTTP_PROXY: "http://api.openai.com/",
  HTTPS_PROXY: "https://api.openai.com/",
} as const;
const LOOPBACK_PROXY_BYPASS = ["localhost", "127.0.0.1", "::1"];

export type SystemProxyResolver = (url: string) => Promise<string>;

export async function resolveSystemProxyEnvironment(
  resolveProxy: SystemProxyResolver,
): Promise<NodeJS.ProcessEnv> {
  try {
    const [httpRules, httpsRules] = await Promise.all([
      resolveProxy(SYSTEM_PROXY_URLS.HTTP_PROXY),
      resolveProxy(SYSTEM_PROXY_URLS.HTTPS_PROXY),
    ]);
    const httpProxy = firstProxyURL(httpRules);
    const httpsProxy = firstProxyURL(httpsRules);
    if (!httpProxy && !httpsProxy) {
      return {};
    }

    const env: NodeJS.ProcessEnv = {
      NO_PROXY: LOOPBACK_PROXY_BYPASS.join(","),
      no_proxy: LOOPBACK_PROXY_BYPASS.join(","),
    };
    setProxyPair(env, "HTTP_PROXY", "http_proxy", httpProxy);
    setProxyPair(env, "HTTPS_PROXY", "https_proxy", httpsProxy);

    const allProxy = commonSocksProxy(httpProxy, httpsProxy);
    if (allProxy) {
      setProxyPair(env, "ALL_PROXY", "all_proxy", allProxy);
    }
    return env;
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

function setProxyPair(
  env: NodeJS.ProcessEnv,
  upperKey: string,
  lowerKey: string,
  value: string | undefined,
): void {
  if (!value) {
    return;
  }
  env[upperKey] = value;
  env[lowerKey] = value;
}

function commonSocksProxy(
  httpProxy: string | undefined,
  httpsProxy: string | undefined,
): string | undefined {
  if (!httpProxy || httpProxy !== httpsProxy || !httpProxy.startsWith("socks")) {
    return undefined;
  }
  return httpProxy;
}

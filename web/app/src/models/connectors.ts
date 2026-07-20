export const GITHUB_CONNECTOR_PROVIDER = "github";
export const GITLAB_CONNECTOR_PROVIDER = "gitlab";
export const DEFAULT_GITHUB_CONNECTOR_SCOPES = ["repo", "read:user", "user:email"] as const;

export type ConnectorAccount = {
  avatar_url: string;
  email: string;
  html_url: string;
  id: number;
  login: string;
  name: string;
};

export type ConnectorStatus = {
  account: ConnectorAccount | null;
  app_manageable: boolean;
  callback_url: string;
  client_id: string;
  client_secret_set: boolean;
  base_url: string;
  access_token_set: boolean;
  configured: boolean;
  connected: boolean;
  connected_at: string;
  name: string;
  oauth_pending: boolean;
  provider: string;
  scopes: string[];
  updated_at: string;
};

export type GitLabConnectorConfigDraft = {
  base_url: string;
  access_token: string;
};

export type ConnectorConfigDraft = {
  client_id: string;
  client_secret: string;
  scopes: string[];
};

export type OAuthStartResponse = {
  authorization_url: string;
  provider: string;
};

export type AppInstallStartResponse = {
  install_url: string;
  provider: string;
};

export function emptyGitHubConnectorStatus(): ConnectorStatus {
  return {
    account: null,
    app_manageable: false,
    callback_url: "",
    client_id: "",
    client_secret_set: false,
    base_url: "",
    access_token_set: false,
    configured: false,
    connected: false,
    connected_at: "",
    name: "GitHub",
    oauth_pending: false,
    provider: GITHUB_CONNECTOR_PROVIDER,
    scopes: [...DEFAULT_GITHUB_CONNECTOR_SCOPES],
    updated_at: "",
  };
}

export function emptyGitLabConnectorStatus(): ConnectorStatus {
  return {
    ...emptyGitHubConnectorStatus(),
    name: "GitLab",
    provider: GITLAB_CONNECTOR_PROVIDER,
    scopes: [],
  };
}

export function normalizeConnectorStatus(source: unknown): ConnectorStatus {
  const item = asRecord(source);
  const provider = cleanString(item?.provider);
  const base = provider === GITLAB_CONNECTOR_PROVIDER ? emptyGitLabConnectorStatus() : emptyGitHubConnectorStatus();
  if (!item) {
    return base;
  }
  const connected = Boolean(item.connected);
  const scopes = normalizeScopes(item.scopes);
  const account = connected ? normalizeConnectorAccount(item.account) : null;
  return {
    ...base,
    account,
    app_manageable: Boolean(item.app_manageable),
    callback_url: cleanString(item.callback_url),
    client_id: cleanString(item.client_id),
    client_secret_set: Boolean(item.client_secret_set),
    base_url: cleanString(item.base_url),
    access_token_set: Boolean(item.access_token_set),
    configured: Boolean(item.configured),
    connected,
    connected_at: cleanString(item.connected_at),
    name: cleanString(item.name) || base.name,
    oauth_pending: Boolean(item.oauth_pending),
    provider: cleanString(item.provider) || base.provider,
    scopes: scopes.length > 0 ? scopes : base.scopes,
    updated_at: cleanString(item.updated_at),
  };
}

export function normalizeConnectorList(source: unknown): ConnectorStatus[] {
  const rawItems = Array.isArray(source) ? source : asRecord(source)?.connectors;
  if (!Array.isArray(rawItems)) {
    return [];
  }
  return rawItems
    .filter((item) => {
      const provider = cleanString(asRecord(item)?.provider);
      return provider === GITHUB_CONNECTOR_PROVIDER || provider === GITLAB_CONNECTOR_PROVIDER;
    })
    .map((item) => normalizeConnectorStatus(item));
}

export function gitLabConnectorDraftFromStatus(status: ConnectorStatus | null | undefined): GitLabConnectorConfigDraft {
  return { base_url: cleanString(status?.base_url), access_token: "" };
}

export function normalizeGitLabConnectorConfigDraft(source: GitLabConnectorConfigDraft): GitLabConnectorConfigDraft {
  return { base_url: cleanString(source.base_url).replace(/\/+$/, ""), access_token: cleanString(source.access_token) };
}

export function normalizeOAuthStartResponse(source: unknown): OAuthStartResponse {
  const item = asRecord(source);
  if (!item) {
    return { authorization_url: "", provider: "" };
  }
  return {
    authorization_url: cleanString(item.authorization_url),
    provider: cleanString(item.provider),
  };
}

export function normalizeAppInstallStartResponse(source: unknown): AppInstallStartResponse {
  const item = asRecord(source);
  if (!item) {
    return { install_url: "", provider: "" };
  }
  return {
    install_url: cleanString(item.install_url),
    provider: cleanString(item.provider),
  };
}

export function githubConnectorDraftFromStatus(status: ConnectorStatus | null | undefined): ConnectorConfigDraft {
  return {
    client_id: cleanString(status?.client_id),
    client_secret: "",
    scopes:
      normalizeScopes(status?.scopes).length > 0
        ? normalizeScopes(status?.scopes)
        : [...DEFAULT_GITHUB_CONNECTOR_SCOPES],
  };
}

export function connectorScopesText(scopes: readonly string[] | null | undefined): string {
  return normalizeScopes(scopes).join(" ");
}

export function normalizeConnectorConfigDraft(source: ConnectorConfigDraft): ConnectorConfigDraft {
  return {
    client_id: cleanString(source.client_id),
    client_secret: cleanString(source.client_secret),
    scopes: normalizeScopes(source.scopes),
  };
}

export function splitConnectorScopes(value: string): string[] {
  return normalizeScopes(value.split(/\s+/));
}

function normalizeConnectorAccount(source: unknown): ConnectorAccount | null {
  const item = asRecord(source);
  if (!item) {
    return null;
  }
  const login = cleanString(item.login);
  if (!login) {
    return null;
  }
  return {
    avatar_url: cleanString(item.avatar_url),
    email: cleanString(item.email),
    html_url: cleanString(item.html_url),
    id: typeof item.id === "number" && Number.isFinite(item.id) ? item.id : Number(cleanString(item.id)) || 0,
    login,
    name: cleanString(item.name),
  };
}

function normalizeScopes(source: unknown): string[] {
  if (!Array.isArray(source)) {
    return [];
  }
  const seen = new Set<string>();
  const scopes: string[] = [];
  for (const item of source) {
    const scope = cleanString(item);
    if (!scope || seen.has(scope)) {
      continue;
    }
    seen.add(scope);
    scopes.push(scope);
  }
  return scopes;
}

function cleanString(value: unknown): string {
  return String(value ?? "").trim();
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
}

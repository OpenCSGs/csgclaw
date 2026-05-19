export type Translator = (key: string) => string;

export type APIKeyProfile = {
  api_key_preview?: string | null;
  api_key_set?: boolean | null;
};

export type ProfileURLDraft = {
  base_url?: string | null;
  provider?: string | null;
};

export type CLIProxyAuthStatus = {
  authenticated?: boolean;
  message?: string | null;
};

export type EnvKeyValueRow = {
  key: string;
  value: string;
};

export type AgentCreateProgressStep = {
  label: string;
  target: number;
};

export type AgentCreateProgressState = {
  index?: number;
  percent?: number;
  startedAt?: number;
  status?: "running" | "failed" | "done" | string;
  steps?: AgentCreateProgressStep[];
};

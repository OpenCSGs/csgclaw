import { formatProviderLabel, normalizeAuthProviderName, providerNeedsAuth } from "@/models/agents";
import { Button } from "@/components/ui";
import type { CLIProxyAuthStatus, Translator } from "./types";

export type CLIProxyAuthControlProps = {
  busy?: boolean;
  onLogin?: (provider: string) => void;
  provider?: string | null;
  status?: CLIProxyAuthStatus | null;
  t: Translator;
};

export function CLIProxyAuthControl({ provider, t, status, busy, onLogin }: CLIProxyAuthControlProps) {
  const normalized = normalizeAuthProviderName(provider);
  if (!providerNeedsAuth(normalized)) {
    return null;
  }
  const connected = Boolean(status?.authenticated);
  const message = connected
    ? `${formatProviderLabel(normalized)} ${t("authConnected")}`
    : status?.message || `${formatProviderLabel(normalized)} ${t("authMissing")}`;
  return (
    <div className={`auth-status-row ${connected ? "connected" : "missing"}`}>
      <span className="auth-status-dot" aria-hidden="true"></span>
      <span className="auth-status-message">{message}</span>
      {connected ? null : (
        <Button className="secondary-button compact" disabled={busy || !onLogin} onClick={() => onLogin?.(normalized)}>
          {busy ? t("authConnecting") : `${t("authConnect")} ${formatProviderLabel(normalized)}`}
        </Button>
      )}
    </div>
  );
}

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  disconnectGitHubConnectorRequest,
  fetchConnectors,
  saveGitHubConnectorConfigRequest,
  startGitHubConnectorAppInstallRequest,
  startGitHubConnectorOAuthRequest,
} from "@/api/connectors";
import { errorMessage } from "@/api/client";
import {
  GITHUB_CONNECTOR_PROVIDER,
  emptyGitHubConnectorStatus,
  normalizeAppInstallStartResponse,
  normalizeConnectorList,
  normalizeConnectorStatus,
  normalizeOAuthStartResponse,
} from "@/models/connectors";
import type { ConnectorConfigDraft, ConnectorStatus } from "@/models/connectors";
import type { TranslateFn } from "@/models/conversations";
import { workspaceQueryKeys } from "./workspaceQueries";

const GITHUB_CONNECTOR_LOGIN_PENDING_STORAGE_KEY = "csgclaw.connectors.github.loginPending";
const LOGIN_POLL_INTERVAL_MS = 2000;
const LOGIN_POLL_TIMEOUT_MS = 120000;

export type ConnectorBusyAction = "connect" | "disconnect" | "manage" | "save" | "";

export type ConnectorController = {
  busy: boolean;
  busyAction: ConnectorBusyAction;
  connectGitHub: () => Promise<void>;
  disconnectGitHub: () => Promise<void>;
  error: string;
  github: ConnectorStatus;
  manageGitHub: () => Promise<void>;
  pending: boolean;
  refresh: () => Promise<ConnectorStatus[]>;
  saveGitHubConfig: (draft: ConnectorConfigDraft) => Promise<void>;
};

export function useConnectorController(t: TranslateFn): ConnectorController {
  const queryClient = useQueryClient();
  const [busyAction, setBusyAction] = useState<ConnectorBusyAction>("");
  const [connectorError, setConnectorError] = useState("");
  const [loginPending, setLoginPending] = useState(false);

  const connectorsQuery = useQuery({
    queryKey: workspaceQueryKeys.connectors(),
    queryFn: fetchNormalizedConnectors,
    retry: 0,
  });

  const github = useMemo(() => {
    return (
      connectorsQuery.data?.find((item) => item.provider === GITHUB_CONNECTOR_PROVIDER) ?? emptyGitHubConnectorStatus()
    );
  }, [connectorsQuery.data]);

  const setGitHubStatus = useCallback(
    (next: ConnectorStatus) => {
      queryClient.setQueryData(workspaceQueryKeys.connectors(), (current: ConnectorStatus[] | undefined) => {
        const statuses = Array.isArray(current) ? current : [];
        const withoutGitHub = statuses.filter((item) => item.provider !== GITHUB_CONNECTOR_PROVIDER);
        return [next, ...withoutGitHub];
      });
    },
    [queryClient],
  );

  const refresh = useCallback(async () => {
    return queryClient.fetchQuery({
      queryKey: workspaceQueryKeys.connectors(),
      queryFn: fetchNormalizedConnectors,
      retry: 0,
    });
  }, [queryClient]);

  const saveGitHubConfig = useCallback(
    async (draft: ConnectorConfigDraft) => {
      if (busyAction) {
        return;
      }
      setBusyAction("save");
      setConnectorError("");
      try {
        await queryClient.cancelQueries({ queryKey: workspaceQueryKeys.connectors() });
        const next = normalizeConnectorStatus(await saveGitHubConnectorConfigRequest(draft));
        setGitHubStatus(next);
      } catch (err) {
        setConnectorError(errorMessage(err, t("connectorSaveFailed")));
      } finally {
        setBusyAction("");
      }
    },
    [busyAction, queryClient, setGitHubStatus, t],
  );

  const connectGitHub = useCallback(async () => {
    if (busyAction) {
      return;
    }
    setBusyAction("connect");
    setConnectorError("");
    const authWindow = openBlankWindow();
    try {
      const start = normalizeOAuthStartResponse(await startGitHubConnectorOAuthRequest(window.location.href));
      if (!start.authorization_url) {
        throw new Error(t("connectorOAuthURLMissing"));
      }
      if (!navigateWindow(authWindow, start.authorization_url)) {
        clearPendingGitHubAuth();
        setLoginPending(false);
        setConnectorError(t("connectorOAuthPopupBlocked"));
        return;
      }
      setGitHubStatus({ ...github, configured: true, oauth_pending: true });
      markPendingGitHubAuth();
      setLoginPending(true);
    } catch (err) {
      closeWindow(authWindow);
      clearPendingGitHubAuth();
      setLoginPending(false);
      setConnectorError(errorMessage(err, t("connectorConnectFailed")));
    } finally {
      setBusyAction("");
    }
  }, [busyAction, github, setGitHubStatus, t]);

  const disconnectGitHub = useCallback(async () => {
    if (busyAction) {
      return;
    }
    setBusyAction("disconnect");
    setConnectorError("");
    try {
      await queryClient.cancelQueries({ queryKey: workspaceQueryKeys.connectors() });
      const next = normalizeConnectorStatus(await disconnectGitHubConnectorRequest());
      setGitHubStatus(next);
      clearPendingGitHubAuth();
      setLoginPending(false);
    } catch (err) {
      setConnectorError(errorMessage(err, t("connectorDisconnectFailed")));
    } finally {
      setBusyAction("");
    }
  }, [busyAction, queryClient, setGitHubStatus, t]);

  const manageGitHub = useCallback(async () => {
    if (busyAction) {
      return;
    }
    setBusyAction("manage");
    setConnectorError("");
    const manageWindow = openBlankWindow();
    try {
      const start = normalizeAppInstallStartResponse(await startGitHubConnectorAppInstallRequest());
      if (!start.install_url) {
        throw new Error(t("connectorManageURLMissing"));
      }
      if (!navigateWindow(manageWindow, start.install_url)) {
        setConnectorError(t("connectorManagePopupBlocked"));
        return;
      }
    } catch (err) {
      closeWindow(manageWindow);
      setConnectorError(errorMessage(err, t("connectorManageFailed")));
    } finally {
      setBusyAction("");
    }
  }, [busyAction, t]);

  useEffect(() => {
    if (!hasPendingGitHubAuth()) {
      return;
    }
    if (github.connected) {
      clearPendingGitHubAuth();
      setLoginPending(false);
      setConnectorError("");
      return;
    }
    if (!connectorsQuery.isLoading && !connectorsQuery.isFetching && !github.oauth_pending) {
      clearPendingGitHubAuth();
      setLoginPending(false);
      return;
    }
    if (github.oauth_pending) {
      setLoginPending(true);
    }
  }, [connectorsQuery.isFetching, connectorsQuery.isLoading, github.connected, github.oauth_pending]);

  useEffect(() => {
    if (!loginPending) {
      return undefined;
    }
    const startedAt = Date.now();
    let cancelled = false;

    async function pollStatus() {
      try {
        const next = await refresh();
        if (cancelled) {
          return;
        }
        const githubStatus = next.find((item) => item.provider === GITHUB_CONNECTOR_PROVIDER);
        if (githubStatus?.connected) {
          clearPendingGitHubAuth();
          setLoginPending(false);
          setConnectorError("");
          return;
        }
      } catch (_) {
        // The callback can still be in flight, so transient errors keep polling until timeout.
      }
      if (!cancelled && Date.now() - startedAt >= LOGIN_POLL_TIMEOUT_MS) {
        clearPendingGitHubAuth();
        setLoginPending(false);
        setConnectorError(t("connectorLoginTimedOut"));
      }
    }

    const firstPoll = window.setTimeout(pollStatus, 1000);
    const interval = window.setInterval(pollStatus, LOGIN_POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(firstPoll);
      window.clearInterval(interval);
    };
  }, [loginPending, refresh, t]);

  return {
    busy: Boolean(busyAction),
    busyAction,
    connectGitHub,
    disconnectGitHub,
    error:
      connectorError ||
      (connectorsQuery.isError ? errorMessage(connectorsQuery.error, t("connectorStatusFailed")) : ""),
    github,
    manageGitHub,
    pending: loginPending,
    refresh,
    saveGitHubConfig,
  };
}

async function fetchNormalizedConnectors(): Promise<ConnectorStatus[]> {
  return normalizeConnectorList(await fetchConnectors());
}

function markPendingGitHubAuth() {
  try {
    window.sessionStorage.setItem(GITHUB_CONNECTOR_LOGIN_PENDING_STORAGE_KEY, "1");
  } catch (_) {
    // Session storage can be unavailable in restricted browser contexts.
  }
}

function clearPendingGitHubAuth() {
  try {
    window.sessionStorage.removeItem(GITHUB_CONNECTOR_LOGIN_PENDING_STORAGE_KEY);
  } catch (_) {
    // Session storage can be unavailable in restricted browser contexts.
  }
}

function hasPendingGitHubAuth(): boolean {
  try {
    return window.sessionStorage.getItem(GITHUB_CONNECTOR_LOGIN_PENDING_STORAGE_KEY) === "1";
  } catch (_) {
    return false;
  }
}

function openBlankWindow(): Window | null {
  try {
    return window.open("about:blank", "_blank");
  } catch (_) {
    return null;
  }
}

function navigateWindow(targetWindow: Window | null, targetURL: string): boolean {
  if (targetWindow) {
    try {
      targetWindow.opener = null;
    } catch (_) {
      // Some browser contexts make opener read-only.
    }
    try {
      targetWindow.location.href = targetURL;
      return true;
    } catch (_) {
      closeWindow(targetWindow);
    }
  }
  try {
    const fallbackWindow = window.open(targetURL, "_blank");
    if (!fallbackWindow) {
      return false;
    }
    try {
      fallbackWindow.opener = null;
    } catch (_) {
      // Some browser contexts make opener read-only.
    }
    return true;
  } catch (_) {
    return false;
  }
}

function closeWindow(targetWindow: Window | null) {
  if (!targetWindow) {
    return;
  }
  try {
    targetWindow.close();
  } catch (_) {
    // Closing a blocked or already-navigated window can throw in restricted contexts.
  }
}

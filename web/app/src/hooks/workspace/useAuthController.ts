import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { beginAuthLogin, fetchAuthStatus, logoutAuth } from "@/api/auth";
import { errorMessage } from "@/api/client";
import { emptyAuthStatus, isAuthenticated, normalizeAuthStatus, normalizeLoginResponse } from "@/models/auth";
import type { AuthStatus } from "@/models/auth";
import {
  authEnvironmentDisplayLabel,
  authEnvironmentDraftFromStatus,
  authEnvironmentLoginPayload,
  defaultAuthEnvironmentDraft,
  resolveAuthEnvironmentDraft,
} from "@/models/authEnvironment";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";
import type { TranslateFn } from "@/models/conversations";
import { avatarFallbackText } from "@/shared/avatar";
import { readStoredAuthEnvironmentDraft, writeStoredAuthEnvironmentDraft } from "@/shared/storage/authEnvironment";
import { workspaceQueryKeys } from "./workspaceQueries";

const AUTH_LOGIN_PENDING_STORAGE_KEY = "csgclaw.auth.loginPending";
const LOGIN_POLL_INTERVAL_MS = 2000;
const LOGIN_POLL_TIMEOUT_MS = 120000;

export type AuthNotice = {
  id: string;
  avatar?: string;
  avatarFallback: string;
  title: string;
  message: string;
  type: "login" | "logout";
  tone: "success";
};

export type AuthController = {
  environment: AuthEnvironmentDraft;
  busy: boolean;
  dismissNotice: () => void;
  error: string;
  login: (environment?: AuthEnvironmentDraft) => Promise<void>;
  logout: () => Promise<void>;
  notice: AuthNotice | null;
  pending: boolean;
  setEnvironment: (environment: AuthEnvironmentDraft) => void;
  status: AuthStatus;
};

export function useAuthController(t: TranslateFn): AuthController {
  const queryClient = useQueryClient();
  const [busyAction, setBusyAction] = useState<"login" | "logout" | "">("");
  const [authError, setAuthError] = useState("");
  const [authNotice, setAuthNotice] = useState<AuthNotice | null>(null);
  const [loginPending, setLoginPending] = useState(false);
  const [selectedEnvironment, setSelectedEnvironment] = useState<AuthEnvironmentDraft>(readStoredAuthEnvironmentDraft);

  const statusQuery = useQuery({
    queryKey: workspaceQueryKeys.authStatus(),
    queryFn: fetchNormalizedAuthStatus,
    retry: 0,
  });

  const status = useMemo(() => statusQuery.data ?? emptyAuthStatus(), [statusQuery.data]);
  const environment = useMemo(
    () => authEnvironmentDraftFromStatus(status, selectedEnvironment),
    [selectedEnvironment, status],
  );

  const setEnvironment = useCallback((next: AuthEnvironmentDraft) => {
    setSelectedEnvironment(resolveAuthEnvironmentDraft(next));
  }, []);

  const setStatus = useCallback(
    (next: AuthStatus) => {
      queryClient.setQueryData(workspaceQueryKeys.authStatus(), next);
    },
    [queryClient],
  );

  const refreshStatus = useCallback(async () => {
    return queryClient.fetchQuery({
      queryKey: workspaceQueryKeys.authStatus(),
      queryFn: fetchNormalizedAuthStatus,
      retry: 0,
    });
  }, [queryClient]);

  const login = useCallback(
    async (requestedEnvironment?: AuthEnvironmentDraft) => {
      if (busyAction) {
        return;
      }
      setBusyAction("login");
      setAuthError("");
      try {
        const nextEnvironment = resolveAuthEnvironmentDraft(requestedEnvironment ?? environment);
        setSelectedEnvironment(nextEnvironment);
        writeStoredAuthEnvironmentDraft(nextEnvironment);
        const payload = authEnvironmentLoginPayload(nextEnvironment);
        const loginResp = normalizeLoginResponse(await beginAuthLogin(window.location.href, payload));
        if (!loginResp.login_url) {
          throw new Error(t("csghubLoginURLMissing"));
        }
        markPendingAuthLogin();
        setLoginPending(true);
        window.location.assign(loginResp.login_url);
      } catch (err) {
        clearPendingAuthLogin();
        setLoginPending(false);
        setAuthError(errorMessage(err, t("csghubLoginFailed")));
      } finally {
        setBusyAction("");
      }
    },
    [busyAction, environment, t],
  );

  const logout = useCallback(async () => {
    if (busyAction) {
      return;
    }
    setBusyAction("logout");
    setAuthError("");
    const userLabel = status.user_id || status.user_uuid || t("csghubSignedIn");
    const avatar = status.avatar;
    const avatarFallback = avatarFallbackText(status.user_id, status.user_uuid, t("csghubSignedIn"));
    try {
      const next = normalizeAuthStatus(await logoutAuth());
      setStatus(next);
      void queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.modelProviders() });
      clearPendingAuthLogin();
      setAuthNotice({
        id: `auth-logout-complete-${Date.now()}`,
        avatar,
        avatarFallback,
        title: t("csghubSignedOut"),
        message: t("csghubLogoutCompleted", { user: userLabel }),
        type: "logout",
        tone: "success",
      });
      setLoginPending(false);
    } catch (err) {
      setAuthError(errorMessage(err, t("csghubLogoutFailed")));
    } finally {
      setBusyAction("");
    }
  }, [busyAction, queryClient, setStatus, status.user_id, status.user_uuid, status.avatar, t]);

  useEffect(() => {
    writeStoredAuthEnvironmentDraft(environment);
  }, [environment]);

  useEffect(() => {
    if (!loginPending) {
      return undefined;
    }
    const startedAt = Date.now();
    let cancelled = false;

    async function pollStatus() {
      try {
        const next = await refreshStatus();
        if (cancelled) {
          return;
        }
        if (isAuthenticated(next)) {
          setLoginPending(false);
          setAuthError("");
          return;
        }
      } catch (_) {
        // Keep polling until timeout; transient callback timing is expected.
      }
      if (!cancelled && Date.now() - startedAt >= LOGIN_POLL_TIMEOUT_MS) {
        clearPendingAuthLogin();
        setLoginPending(false);
        setAuthError(t("csghubLoginTimedOut"));
      }
    }

    const firstPoll = window.setTimeout(pollStatus, 1000);
    const interval = window.setInterval(pollStatus, LOGIN_POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(firstPoll);
      window.clearInterval(interval);
    };
  }, [loginPending, refreshStatus, t]);

  useEffect(() => {
    if (!isAuthenticated(status) || !consumePendingAuthLogin()) {
      return;
    }
    const user = status.user_id || status.user_uuid || t("csghubSignedIn");
    const environment = authNoticeEnvironmentLabel(status, t);
    void queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.modelProviders() });
    setLoginPending(false);
    setAuthError("");
    setAuthNotice({
      id: `auth-login-complete-${Date.now()}`,
      avatar: status.avatar,
      avatarFallback: avatarFallbackText(status.user_id, status.user_uuid, t("csghubSignedIn")),
      title: t("csghubSignedIn"),
      message: environment
        ? t("csghubLoginEnvironmentCompleted", { user, environment })
        : t("csghubLoginCompleted", { user }),
      type: "login",
      tone: "success",
    });
  }, [queryClient, status, t]);

  return {
    environment,
    busy: Boolean(busyAction),
    dismissNotice: () => setAuthNotice(null),
    error: authError || (statusQuery.isError ? errorMessage(statusQuery.error, t("csghubStatusFailed")) : ""),
    login,
    logout,
    notice: authNotice,
    pending: loginPending,
    setEnvironment,
    status,
  };
}

async function fetchNormalizedAuthStatus(): Promise<AuthStatus> {
  return normalizeAuthStatus(await fetchAuthStatus());
}

function markPendingAuthLogin() {
  try {
    window.sessionStorage.setItem(AUTH_LOGIN_PENDING_STORAGE_KEY, "1");
  } catch (_) {
    // Session storage can be unavailable in restricted browser contexts.
  }
}

function clearPendingAuthLogin() {
  try {
    window.sessionStorage.removeItem(AUTH_LOGIN_PENDING_STORAGE_KEY);
  } catch (_) {
    // Session storage can be unavailable in restricted browser contexts.
  }
}

function consumePendingAuthLogin(): boolean {
  try {
    const pending = window.sessionStorage.getItem(AUTH_LOGIN_PENDING_STORAGE_KEY) === "1";
    if (pending) {
      window.sessionStorage.removeItem(AUTH_LOGIN_PENDING_STORAGE_KEY);
    }
    return pending;
  } catch (_) {
    return false;
  }
}

function authNoticeEnvironmentLabel(status: AuthStatus, t: TranslateFn): string {
  const customLabel = t("csghubEnvCustom");
  const label = authEnvironmentDisplayLabel(authEnvironmentDraftFromStatus(status), customLabel);
  const defaultLabel = authEnvironmentDisplayLabel(defaultAuthEnvironmentDraft(), customLabel);
  return label && label !== defaultLabel ? label : "";
}

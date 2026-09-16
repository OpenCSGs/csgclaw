import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AuthStatus } from "@/models/auth";
import { isAuthenticated } from "@/models/auth";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";

const OPENCSG_AUTH_ERROR_CODES = new Set(["authentication_error", "auth_required", "invalid_api_key", "unauthorized"]);

type MarkAuthenticationExpiredOptions = {
  openDialog?: boolean;
};

export type OpenCSGAuthGuard = {
  authenticated: boolean;
  closeDialog: () => void;
  dialogOpen: boolean;
  handleAuthenticationError: (error: unknown, options?: MarkAuthenticationExpiredOptions) => boolean;
  login: (environment?: AuthEnvironmentDraft) => Promise<void>;
  markAuthenticationExpired: (options?: MarkAuthenticationExpiredOptions) => void;
  requireAuthentication: () => boolean;
};

export type UseOpenCSGAuthGuardArgs = {
  login: (environment?: AuthEnvironmentDraft) => Promise<void>;
  status: AuthStatus;
};

export function useOpenCSGAuthGuard({ login, status }: UseOpenCSGAuthGuardArgs): OpenCSGAuthGuard {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [authenticationExpired, setAuthenticationExpired] = useState(false);
  const handledAuthenticationErrorsRef = useRef(new WeakSet<object>());
  const invalidatedLoginAtRef = useRef("");
  const loggedInAtRef = useRef(status.logged_in_at);
  loggedInAtRef.current = status.logged_in_at;
  const authenticated = useMemo(
    () => isAuthenticated(status) && !authenticationExpired,
    [authenticationExpired, status],
  );

  const closeDialog = useCallback(() => {
    handledAuthenticationErrorsRef.current = new WeakSet<object>();
    setDialogOpen(false);
  }, []);

  const markAuthenticationExpired = useCallback((options: MarkAuthenticationExpiredOptions = {}) => {
    invalidatedLoginAtRef.current = loggedInAtRef.current;
    setAuthenticationExpired(true);
    if (options.openDialog !== false) {
      setDialogOpen(true);
    }
  }, []);

  const handleAuthenticationError = useCallback(
    (error: unknown, options?: MarkAuthenticationExpiredOptions) => {
      if (!isOpenCSGAuthenticationError(error)) {
        return false;
      }
      const errorObject = error as object;
      if (handledAuthenticationErrorsRef.current.has(errorObject)) {
        return true;
      }
      handledAuthenticationErrorsRef.current.add(errorObject);
      markAuthenticationExpired(options);
      return true;
    },
    [markAuthenticationExpired],
  );

  const requireAuthentication = useCallback(() => {
    if (authenticated) {
      return true;
    }
    setDialogOpen(true);
    return false;
  }, [authenticated]);

  useEffect(() => {
    if (!isAuthenticated(status)) {
      return;
    }
    if (!authenticationExpired) {
      setDialogOpen(false);
      return;
    }
    const loggedInAt = status.logged_in_at;
    if (!loggedInAt || loggedInAt === invalidatedLoginAtRef.current) {
      return;
    }
    invalidatedLoginAtRef.current = "";
    setAuthenticationExpired(false);
    setDialogOpen(false);
  }, [authenticationExpired, status]);

  return useMemo(
    () => ({
      authenticated,
      closeDialog,
      dialogOpen,
      handleAuthenticationError,
      login,
      markAuthenticationExpired,
      requireAuthentication,
    }),
    [
      authenticated,
      closeDialog,
      dialogOpen,
      handleAuthenticationError,
      login,
      markAuthenticationExpired,
      requireAuthentication,
    ],
  );
}

export function isOpenCSGAuthenticationError(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }
  const value = error as { code?: unknown; status?: unknown };
  if (value.status === 401) {
    return true;
  }
  return typeof value.code === "string" && OPENCSG_AUTH_ERROR_CODES.has(value.code.trim().toLowerCase());
}

export function isOpenCSGRuntimeAuthenticationError(message: unknown): boolean {
  if (!message || typeof message !== "object") {
    return false;
  }
  const metadata = (message as { metadata?: unknown }).metadata;
  if (!metadata || typeof metadata !== "object") {
    return false;
  }
  const csgclaw = (metadata as Record<string, unknown>).csgclaw;
  if (!csgclaw || typeof csgclaw !== "object") {
    return false;
  }
  const details = csgclaw as Record<string, unknown>;
  const code = typeof details.error_code === "string" ? details.error_code.trim().toLowerCase() : "";
  return details.runtime_error === true && OPENCSG_AUTH_ERROR_CODES.has(code);
}

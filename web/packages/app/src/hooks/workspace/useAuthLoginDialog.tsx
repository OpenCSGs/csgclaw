import { useState } from "react";
import { OpenCSGConnectionDialog } from "@/components/business/OpenCSGConnectionDialog";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { authEnvironmentDraftFromStatus, defaultAuthEnvironmentDraft } from "@/models/authEnvironment";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";
import { readStoredAuthEnvironmentDraft, writeStoredAuthEnvironmentDraft } from "@/shared/storage/authEnvironment";

export function useAuthLoginDialog({
  closeOnConnect = true,
  onCancel,
}: { closeOnConnect?: boolean; onCancel?: () => void } = {}) {
  const controller = useWorkspaceControllerContext();
  const sidebar = controller.sidebarProps;
  const [connectionOpen, setConnectionOpen] = useState(false);
  const [connectionDraft, setConnectionDraft] = useState<AuthEnvironmentDraft>(defaultAuthEnvironmentDraft);

  const currentDraft = (): AuthEnvironmentDraft => {
    const stored = sidebar?.authEnvironment ?? readStoredAuthEnvironmentDraft();
    return sidebar ? authEnvironmentDraftFromStatus(sidebar.authStatus, stored) : stored;
  };

  function openLoginDialog(draft?: AuthEnvironmentDraft) {
    setConnectionDraft(draft ?? currentDraft());
    setConnectionOpen(true);
  }

  function handleLogin(environment: AuthEnvironmentDraft) {
    writeStoredAuthEnvironmentDraft(environment);
    sidebar?.onAuthEnvironmentChange?.(environment);
    if (closeOnConnect) setConnectionOpen(false);
    void sidebar?.onLogin(environment);
  }

  const loginDialog = sidebar ? (
    <OpenCSGConnectionDialog
      busy={sidebar.authBusy}
      environment={connectionDraft}
      error={closeOnConnect ? undefined : sidebar.authError}
      locale={sidebar.locale}
      open={connectionOpen}
      t={sidebar.t}
      onConnect={handleLogin}
      onOpenChange={(open) => {
        setConnectionOpen(open);
        if (!open) onCancel?.();
      }}
    />
  ) : null;

  return { loginDialog, openLoginDialog };
}

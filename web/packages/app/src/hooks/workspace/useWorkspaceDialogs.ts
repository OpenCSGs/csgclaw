import { useCallback, useMemo } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
  closeDialogSearch,
  openWorkspaceDialog,
  profileDialogSearch,
  settingsDialogSearch,
} from "@/models/workspaceDialogs";
import type { WorkspaceDialogName } from "@/models/workspaceDialogs";

export type UseWorkspaceDialogsResult = {
  current: WorkspaceDialogName | "";
  openSettings: (section?: string) => void;
  openProfile: (humanID: string) => void;
  close: () => void;
};

export function useWorkspaceDialogs(): UseWorkspaceDialogsResult {
  const location = useLocation();
  const navigate = useNavigate();
  const search = location.search;

  const current = useMemo(() => openWorkspaceDialog(search), [search]);

  const openSettings = useCallback(
    (section?: string) => {
      navigate({ search: settingsDialogSearch(search, section) }, { replace: true });
    },
    [navigate, search],
  );

  const openProfile = useCallback(
    (humanID: string) => {
      navigate({ search: profileDialogSearch(search, humanID) }, { replace: true });
    },
    [navigate, search],
  );

  const close = useCallback(() => {
    navigate({ search: closeDialogSearch(search) }, { replace: true });
  }, [navigate, search]);

  return { current, openSettings, openProfile, close };
}

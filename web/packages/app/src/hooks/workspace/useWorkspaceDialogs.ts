import { useCallback, useMemo } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
  closeDialogSearch,
  openWorkspaceDialog,
  profileDialogSearch,
  settingsDialogSearch,
  WorkspaceDialogNames,
} from "@/models/workspaceDialogs";
import type { WorkspaceDialogName } from "@/models/workspaceDialogs";

export type UseWorkspaceDialogsResult = {
  current: WorkspaceDialogName | "";
  openSettings: (section?: string, options?: { mark?: "version" }) => void;
  openProfile: (humanID: string) => void;
  close: () => void;
};

export function useWorkspaceDialogs(): UseWorkspaceDialogsResult {
  const location = useLocation();
  const navigate = useNavigate();
  const search = location.search;

  const current = useMemo(() => openWorkspaceDialog(search), [search]);

  const openSettings = useCallback(
    (section?: string, options?: { mark?: "version" }) => {
      const fromUpdateNotice = section === "version" && options?.mark === "version" && !current;
      navigate(
        { search: settingsDialogSearch(search, section, options?.mark) },
        {
          replace: !fromUpdateNotice,
          state: fromUpdateNotice ? { ...location.state, settingsOpenedFromUpdateNotice: true } : location.state,
        },
      );
    },
    [current, location.state, navigate, search],
  );

  const openProfile = useCallback(
    (humanID: string) => {
      navigate({ search: profileDialogSearch(search, humanID) }, { replace: true });
    },
    [navigate, search],
  );

  const close = useCallback(() => {
    if (current === WorkspaceDialogNames.settings && location.state?.settingsOpenedFromUpdateNotice === true) {
      navigate(-1);
    } else {
      navigate({ search: closeDialogSearch(search) }, { replace: true });
    }
  }, [current, location.state, navigate, search]);

  return { current, openSettings, openProfile, close };
}

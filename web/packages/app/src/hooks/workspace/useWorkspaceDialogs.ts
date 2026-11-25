import { useCallback, useContext, useMemo } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
  closeDialogSearch,
  feedbackDialogSearch,
  openWorkspaceDialog,
  profileDialogSearch,
  settingsDialogSearch,
} from "@/models/workspaceDialogs";
import type { WorkspaceDialogName } from "@/models/workspaceDialogs";

import { FeedbackTriggerContext } from "./workspaceControllerContextValue";

export type UseWorkspaceDialogsResult = {
  current: WorkspaceDialogName | "";
  openSettings: (section?: string, highlight?: boolean) => void;
  feedbackOpen: boolean;
  setFeedbackOpen: (open: boolean, trigger?: HTMLElement | null) => void;
  feedbackTrigger: HTMLElement | null;
  openProfile: (humanID: string) => void;
  close: () => void;
};

export function useWorkspaceDialogs(): UseWorkspaceDialogsResult {
  const location = useLocation();
  const navigate = useNavigate();
  const search = location.search;
  const feedbackTrigger = useContext(FeedbackTriggerContext);

  const current = useMemo(() => openWorkspaceDialog(search), [search]);

  const openSettings = useCallback(
    (section?: string, highlight?: boolean) => {
      navigate({ search: settingsDialogSearch(search, section, highlight) }, { replace: true });
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

  const setFeedbackOpen = useCallback(
    (open: boolean, trigger?: HTMLElement | null) => {
      if (feedbackTrigger) feedbackTrigger.current = open ? (trigger ?? null) : null;
      navigate({ search: feedbackDialogSearch(search, open) }, { replace: true });
    },
    [feedbackTrigger, navigate, search],
  );

  return {
    current,
    openSettings,
    openProfile,
    close,
    feedbackOpen: new URLSearchParams(search).get("feedback") === "1",
    setFeedbackOpen,
    feedbackTrigger: feedbackTrigger?.current ?? null,
  };
}

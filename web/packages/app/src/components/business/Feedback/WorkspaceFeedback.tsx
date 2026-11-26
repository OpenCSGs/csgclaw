import { useEffect, useRef } from "react";
import { useWorkspaceControllerContext, useWorkspaceDialogs } from "@/hooks/workspace";
import { useAuthLoginDialog } from "@/hooks/workspace/useAuthLoginDialog";
import { isAuthenticated } from "@/models/auth";
import { AUTH_ENVIRONMENT_PRESETS, defaultAuthEnvironmentDraft } from "@/models/authEnvironment";
import { githubFeedbackIssueURL } from "@/models/feedback";
import { FeedbackDialog } from "./FeedbackDialog";
import { useFeedback } from "./useFeedback";

export function WorkspaceFeedback() {
  const { t, sidebarProps } = useWorkspaceControllerContext();
  const { feedbackOpen, feedbackTrigger, setFeedbackOpen } = useWorkspaceDialogs();
  const status = sidebarProps?.authStatus;
  const signedIn = isAuthenticated(status) && !sidebarProps?.authExpired;
  const authReady = Boolean(sidebarProps && !sidebarProps.authStatusLoading);
  const legacySite = AUTH_ENVIRONMENT_PRESETS.find((preset) => preset.csgHubBaseURL === status?.base_url);
  const site =
    status?.opencsg_base_url ||
    legacySite?.opencsgBaseURL ||
    status?.base_url ||
    defaultAuthEnvironmentDraft().opencsgBaseURL;
  const account = status?.user_uuid ? `uuid:${status.user_uuid}` : status?.user_id ? `user:${status.user_id}` : "";
  const prompted = useRef(false);
  const loginReturnFocus = useRef<HTMLElement | null>(null);
  const loginCancelled = useRef(false);
  const feedback = useFeedback(t, account && site ? JSON.stringify([site, account]) : "");
  const { loginDialog, openLoginDialog } = useAuthLoginDialog({
    closeOnConnect: false,
    enabled: feedbackOpen && authReady && !signedIn,
    feedbackURL: githubFeedbackIssueURL(sidebarProps?.appVersion ?? "", sidebarProps?.upgradeStatus ?? null),
    onCancel: () => {
      loginCancelled.current = true;
      setFeedbackOpen(false);
    },
    onCloseAutoFocus: (event) => {
      event.preventDefault();
      if (loginCancelled.current) loginReturnFocus.current?.focus({ preventScroll: true });
    },
  });

  useEffect(() => {
    if (!feedbackOpen || signedIn) {
      prompted.current = false;
    } else if (authReady && !prompted.current) {
      prompted.current = true;
      loginCancelled.current = false;
      loginReturnFocus.current = feedbackTrigger;
      openLoginDialog();
    }
  }, [authReady, feedbackOpen, feedbackTrigger, openLoginDialog, signedIn]);

  return (
    <>
      <FeedbackDialog
        {...feedback}
        open={feedbackOpen && signedIn}
        returnFocusTo={feedbackTrigger}
        receipt={feedbackOpen ? feedback.receipt : null}
        site={site}
        t={t}
        onOpenChange={(open) => {
          if (!open) feedback.clearReceipt();
          setFeedbackOpen(open);
        }}
        onDescriptionChange={feedback.changeDescription}
        onAddAttachments={feedback.addAttachments}
        onRemoveAttachment={feedback.removeAttachment}
        onClearError={feedback.clearError}
        onClearRejections={feedback.clearRejections}
        onCheckStatus={feedback.checkStatus}
        onDiscardDraft={feedback.discardDraft}
        onSubmit={() => void feedback.submit()}
      />
      {loginDialog}
    </>
  );
}

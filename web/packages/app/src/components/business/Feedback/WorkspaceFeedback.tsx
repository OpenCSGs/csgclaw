import { useEffect, useRef } from "react";
import { useWorkspaceControllerContext, useWorkspaceDialogs } from "@/hooks/workspace";
import { useAuthLoginDialog } from "@/hooks/workspace/useAuthLoginDialog";
import { isAuthenticated } from "@/models/auth";
import { FeedbackDialog } from "./FeedbackDialog";
import { useFeedback } from "./useFeedback";

export function WorkspaceFeedback() {
  const { t, sidebarProps } = useWorkspaceControllerContext();
  const { feedbackOpen, feedbackTrigger, setFeedbackOpen } = useWorkspaceDialogs();
  const status = sidebarProps?.authStatus;
  const signedIn = isAuthenticated(status) && !sidebarProps?.authExpired;
  const prompted = useRef(false);
  const loginReturnFocus = useRef<HTMLElement | null>(null);
  const loginCancelled = useRef(false);
  const feedback = useFeedback(t, `${status?.user_uuid || ""}:${status?.base_url || ""}`);
  const { loginDialog, openLoginDialog } = useAuthLoginDialog({
    closeOnConnect: false,
    enabled: feedbackOpen && !signedIn,
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
    } else if (sidebarProps && !prompted.current) {
      prompted.current = true;
      loginCancelled.current = false;
      loginReturnFocus.current = feedbackTrigger;
      openLoginDialog();
    }
  }, [feedbackOpen, feedbackTrigger, openLoginDialog, sidebarProps, signedIn]);

  return (
    <>
      <FeedbackDialog
        {...feedback}
        open={feedbackOpen && signedIn}
        returnFocusTo={feedbackTrigger}
        receipt={feedbackOpen ? feedback.receipt : null}
        t={t}
        onOpenChange={(open) => {
          if (!open) feedback.clearReceipt();
          setFeedbackOpen(open);
        }}
        onDescriptionChange={feedback.changeDescription}
        onAddAttachments={feedback.addAttachments}
        onRemoveAttachment={feedback.removeAttachment}
        onSubmit={() => void feedback.submit()}
      />
      {loginDialog}
    </>
  );
}

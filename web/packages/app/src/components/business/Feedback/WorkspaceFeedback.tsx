import { useEffect, useRef } from "react";
import { useWorkspaceControllerContext, useWorkspaceDialogs } from "@/hooks/workspace";
import { useAuthLoginDialog } from "@/hooks/workspace/useAuthLoginDialog";
import { isAuthenticated } from "@/models/auth";
import { FeedbackDialog } from "./FeedbackDialog";
import { useFeedback } from "./useFeedback";

export function WorkspaceFeedback() {
  const { t, sidebarProps } = useWorkspaceControllerContext();
  const { feedbackOpen, setFeedbackOpen } = useWorkspaceDialogs();
  const status = sidebarProps?.authStatus;
  const signedIn = isAuthenticated(status) && !sidebarProps?.authExpired;
  const prompted = useRef(false);
  const feedback = useFeedback(t, `${status?.user_uuid || ""}:${status?.base_url || ""}`);
  const { loginDialog, openLoginDialog } = useAuthLoginDialog({
    closeOnConnect: false,
    onCancel: () => setFeedbackOpen(false),
  });

  useEffect(() => {
    if (!feedbackOpen || signedIn) {
      prompted.current = false;
    } else if (sidebarProps && !prompted.current) {
      prompted.current = true;
      openLoginDialog();
    }
  }, [feedbackOpen, openLoginDialog, sidebarProps, signedIn]);

  return (
    <>
      <FeedbackDialog
        {...feedback}
        open={feedbackOpen && signedIn}
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
      {feedbackOpen && !signedIn ? loginDialog : null}
    </>
  );
}

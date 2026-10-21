import { forwardRef, useEffect, useRef, useState } from "react";
import {
  Button,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import { isNotificationBotAgent, agentDeleteConfirmationMessage } from "@/models/agents";
import type { AgentLike } from "@/models/agents";
import { AgentDetailPane } from "../AgentDetailPane";
import type { AgentDetailPaneHandle, AgentDetailPaneProps } from "../AgentDetailPane";
import { NotificationParticipantDetailPane } from "../NotificationParticipantDetailPane";

export const AgentView = forwardRef<AgentDetailPaneHandle, AgentDetailPaneProps>(function AgentView(props, ref) {
  const [deletePendingAgent, setDeletePendingAgent] = useState<AgentLike | null>(null);
  const [deleteBlock, setDeleteBlock] = useState<{ message: string; onResolve?: () => void } | null>(null);
  const [deleteChecking, setDeleteChecking] = useState(false);
  const deleteRequest = useRef(0);
  const deleteConfirmMessage = deletePendingAgent ? agentDeleteConfirmationMessage(deletePendingAgent, props.t) : "";

  useEffect(() => {
    setDeletePendingAgent(null);
    deleteRequest.current += 1;
    setDeleteBlock(null);
    setDeleteChecking(false);
  }, [props.item?.id]);

  function closeDelete() {
    deleteRequest.current += 1;
    setDeletePendingAgent(null);
  }

  async function requestDelete(item: AgentLike) {
    const request = ++deleteRequest.current;
    setDeletePendingAgent(item);
    setDeleteBlock(null);
    setDeleteChecking(Boolean(props.onCheckDelete));
    if (!props.onCheckDelete) return;
    try {
      const block = await props.onCheckDelete(item);
      if (request === deleteRequest.current) setDeleteBlock(block);
    } catch {
      if (request === deleteRequest.current) setDeleteBlock({ message: props.t("agentDeleteCheckFailed") });
    } finally {
      if (request === deleteRequest.current) setDeleteChecking(false);
    }
  }

  async function confirmDelete() {
    const item = deletePendingAgent;
    if (!item || deleteChecking || deleteBlock) {
      return;
    }
    closeDelete();
    await Promise.resolve(props.onDelete(item));
  }

  const sharedProps = {
    ...props,
    onDelete: requestDelete,
  };

  return (
    <>
      {isNotificationBotAgent(props.item) ? (
        <NotificationParticipantDetailPane {...sharedProps} />
      ) : (
        <AgentDetailPane ref={ref} {...sharedProps} />
      )}
      <DialogRoot
        open={Boolean(deletePendingAgent)}
        onOpenChange={(open) => {
          if (!open) {
            closeDelete();
          }
        }}
      >
        <DialogContent
          className="agent-delete-dialog"
          overlayClassName="agent-delete-backdrop"
          portalContainer={props.dialogPortalContainer}
        >
          <DialogHeader className="agent-delete-header">
            <div className="agent-delete-copy">
              <DialogTitle>{props.t(deleteBlock ? "agentDeleteBlockedTitle" : "agentDelete")}</DialogTitle>
              <DialogDescription className="agent-delete-description">
                {deleteChecking ? props.t("agentDeleteChecking") : deleteBlock?.message || deleteConfirmMessage}
              </DialogDescription>
            </div>
            <DialogCloseButton label={props.t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <div className="agent-delete-actions">
            <Button className="agent-delete-button" variant="secondaryGray" size="sm" onClick={closeDelete}>
              {props.t("cancel")}
            </Button>
            {deleteBlock ? (
              deleteBlock.onResolve && (
                <Button
                  className="agent-delete-button"
                  variant="primary"
                  size="sm"
                  onClick={() => {
                    closeDelete();
                    deleteBlock.onResolve?.();
                  }}
                >
                  {props.t("agentDeleteResolve")}
                </Button>
              )
            ) : (
              <Button
                className="agent-delete-button"
                variant="danger"
                size="sm"
                disabled={deleteChecking}
                onClick={confirmDelete}
              >
                {props.t("agentDelete")}
              </Button>
            )}
          </div>
        </DialogContent>
      </DialogRoot>
    </>
  );
});

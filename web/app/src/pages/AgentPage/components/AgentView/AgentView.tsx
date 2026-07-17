import { forwardRef, useEffect, useState } from "react";
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
  const deleteConfirmMessage = deletePendingAgent ? agentDeleteConfirmationMessage(deletePendingAgent, props.t) : "";

  useEffect(() => {
    setDeletePendingAgent(null);
  }, [props.item?.id]);

  function requestDelete(item: AgentLike) {
    setDeletePendingAgent(item);
  }

  async function confirmDelete() {
    const item = deletePendingAgent;
    if (!item) {
      return;
    }
    setDeletePendingAgent(null);
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
            setDeletePendingAgent(null);
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
              <DialogTitle>{props.t("agentDelete")}</DialogTitle>
              <DialogDescription className="agent-delete-description">{deleteConfirmMessage}</DialogDescription>
            </div>
            <DialogCloseButton label={props.t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <div className="agent-delete-actions">
            <Button
              className="agent-delete-button"
              variant="secondaryGray"
              size="sm"
              onClick={() => setDeletePendingAgent(null)}
            >
              {props.t("cancel")}
            </Button>
            <Button className="agent-delete-button" variant="danger" size="sm" onClick={confirmDelete}>
              {props.t("agentDelete")}
            </Button>
          </div>
        </DialogContent>
      </DialogRoot>
    </>
  );
});

import { useAgentArchiveImpact } from "@/hooks/workspace/useAgentArchiveImpact";
import { forwardRef, useEffect, useRef, useState } from "react";
import { Archive } from "lucide-react";
import {
  Button,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from "@/components/ui";
import { isNotificationBotAgent, agentDeleteConfirmationMessage } from "@/models/agents";
import type { AgentLike } from "@/models/agents";
import { AgentDetailPane } from "../AgentDetailPane";
import type { AgentDetailPaneHandle, AgentDetailPaneProps } from "../AgentDetailPane";
import { NotificationParticipantDetailPane } from "../NotificationParticipantDetailPane";
import styles from "./AgentView.module.css";

export const AgentView = forwardRef<AgentDetailPaneHandle, AgentDetailPaneProps>(function AgentView(props, ref) {
  const [deletePendingAgent, setDeletePendingAgent] = useState<AgentLike | null>(null);
  const [deleteBlock, setDeleteBlock] = useState<{ message: string; onResolve?: () => void } | null>(null);
  const [deleteChecking, setDeleteChecking] = useState(false);
  const deleteRequest = useRef(0);
  const returnFocus = useRef<HTMLElement | null>(null);
  const archiving = Boolean(deletePendingAgent && !isNotificationBotAgent(deletePendingAgent));
  const impact = useAgentArchiveImpact(archiving ? String(deletePendingAgent?.id || "") : null);
  const deleteConfirmMessage = deletePendingAgent
    ? archiving
      ? props.t("agentArchiveConfirm", { name: String(deletePendingAgent.name || deletePendingAgent.id) })
      : agentDeleteConfirmationMessage(deletePendingAgent, props.t)
    : "";

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

  async function requestDelete(item: AgentLike, trigger?: HTMLElement | null) {
    returnFocus.current = trigger || (document.activeElement instanceof HTMLElement ? document.activeElement : null);
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
    if (!item || deleteChecking || deleteBlock || (archiving && (impact.loading || impact.failed))) {
      return;
    }
    closeDelete();
    await Promise.resolve(props.onDelete(item));
  }

  const sharedProps = {
    ...props,
    onDelete: requestDelete,
  };
  const Actions = archiving ? DialogFooter : "div";

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
          className={archiving ? styles.archiveDialog : "agent-delete-dialog"}
          overlayClassName={archiving ? undefined : "agent-delete-backdrop"}
          portalContainer={props.dialogPortalContainer}
          onCloseAutoFocus={(event) => {
            if (returnFocus.current?.isConnected) {
              event.preventDefault();
              returnFocus.current.focus();
            }
          }}
        >
          <DialogHeader className={archiving ? undefined : "agent-delete-header"}>
            <div className={archiving ? "min-w-0" : "agent-delete-copy"}>
              <DialogTitle>
                {props.t(deleteBlock ? "agentDeleteBlockedTitle" : archiving ? "agentArchiveTitle" : "agentDelete", {
                  name: String(deletePendingAgent?.name || deletePendingAgent?.id || ""),
                })}
              </DialogTitle>
              <DialogDescription className={archiving ? undefined : "agent-delete-description"}>
                {deleteChecking ? props.t("agentDeleteChecking") : deleteBlock?.message || deleteConfirmMessage}
              </DialogDescription>
            </div>
            <DialogCloseButton label={props.t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          {archiving && !deleteBlock && !deleteChecking && (
            <DialogBody>
              {impact.loading && <p role="status">{props.t("agentArchiveImpactLoading")}</p>}
              {impact.failed && (
                <div className="flex flex-col gap-3">
                  <p role="alert">{props.t("agentArchiveImpactFailed")}</p>
                  <Button variant="secondaryGray" size="sm" onClick={impact.retry}>
                    {props.t("retry")}
                  </Button>
                </div>
              )}
              {impact.data && (
                <ul className={styles.impactList}>
                  <li>{props.t("agentArchiveSchedules", { count: impact.data.scheduled_tasks })}</li>
                  <li>{props.t("agentArchiveRunning")}</li>
                  <li>{props.t("agentArchiveTeams", { count: impact.data.teams })}</li>
                  <li>{props.t("agentArchiveSpaces", { count: impact.data.spaces })}</li>
                  <li>{props.t("agentArchiveChats", { count: impact.data.direct_chats })}</li>
                  <li className={styles.recoverable}>{props.t("agentArchiveRestorePath")}</li>
                </ul>
              )}
            </DialogBody>
          )}
          <Actions className={archiving ? undefined : "agent-delete-actions"}>
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
                variant={archiving ? "primary" : "danger"}
                size="sm"
                disabled={deleteChecking || (archiving && (impact.loading || impact.failed))}
                onClick={confirmDelete}
              >
                {archiving && <Archive size={15} aria-hidden="true" />}
                {props.t(archiving ? "agentArchiveConfirmAction" : "agentDelete")}
              </Button>
            )}
          </Actions>
        </DialogContent>
      </DialogRoot>
    </>
  );
});

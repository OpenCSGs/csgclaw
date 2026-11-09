import roomDangerDialogStyles from "./RoomDangerDialog.module.css";
import agentLogsDialogStyles from "./AgentLogsDialog.module.css";
import { useLayoutEffect, useRef } from "react";
import { RefreshCw } from "lucide-react";
import {
  IconButton,
  InlineError,
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Tooltip,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { localizeAgentLogTimestamps } from "@/models/agentLogs";
import type { VoidOrPromise } from "./types";

export type ConversationRoomDangerConfirmDialogProps = {
  busy?: boolean;
  cancelLabel: string;
  closeLabel: string;
  confirmLabel: string;
  description: string;
  error?: string;
  open: boolean;
  title: string;
  onConfirm: () => VoidOrPromise;
  onOpenChange: (open: boolean) => void;
};

export function ConversationRoomDangerConfirmDialog({
  busy = false,
  cancelLabel,
  closeLabel,
  confirmLabel,
  description,
  error = "",
  open,
  title,
  onConfirm,
  onOpenChange,
}: ConversationRoomDangerConfirmDialogProps) {
  return (
    <DialogRoot open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={roomDangerDialogStyles["room-danger-dialog"]}
        overlayClassName={roomDangerDialogStyles["room-danger-backdrop"]}
      >
        <DialogHeader className={roomDangerDialogStyles["room-danger-header"]}>
          <div className={roomDangerDialogStyles["room-danger-copy"]}>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </div>
          <DialogCloseButton
            className={roomDangerDialogStyles["room-danger-close"]}
            label={closeLabel}
            size="sm"
            variant="tertiaryGray"
          />
        </DialogHeader>
        {error ? (
          <div className={roomDangerDialogStyles["room-danger-error"]} role="alert">
            {error}
          </div>
        ) : null}
        <div className={roomDangerDialogStyles["room-danger-actions"]}>
          <Button
            className={roomDangerDialogStyles["room-danger-button"]}
            disabled={busy}
            size="sm"
            variant="secondaryGray"
            onClick={() => onOpenChange(false)}
          >
            {cancelLabel}
          </Button>
          <Button
            className={roomDangerDialogStyles["room-danger-button"]}
            disabled={busy}
            size="sm"
            variant="danger"
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </div>
      </DialogContent>
    </DialogRoot>
  );
}

export type ConversationAgentLogsDialogProps = {
  agentName: string;
  content: string;
  error: string;
  loading: boolean;
  onClose: () => void;
  onRefresh: () => VoidOrPromise;
  t: TranslateFn;
};

export function ConversationAgentLogsDialog({
  agentName,
  content,
  error,
  loading,
  t,
  onClose,
  onRefresh,
}: ConversationAgentLogsDialogProps) {
  const logsViewerRef = useRef<HTMLPreElement | null>(null);
  const displayContent = localizeAgentLogTimestamps(content) || (loading ? t("agentLogsLoading") : t("agentLogsEmpty"));

  useLayoutEffect(() => {
    const viewer = logsViewerRef.current;
    if (!viewer) {
      return;
    }
    viewer.scrollTop = viewer.scrollHeight;
  }, [content, error, loading]);

  return (
    <DialogRoot
      open={true}
      onOpenChange={(open) => {
        if (!open) {
          onClose();
        }
      }}
    >
      <DialogContent
        className={agentLogsDialogStyles["agent-logs-modal"]}
        overlayClassName={agentLogsDialogStyles["agent-logs-backdrop"]}
      >
        <DialogHeader className={agentLogsDialogStyles["agent-logs-header"]}>
          <div>
            <DialogTitle>{t("agentLogsTitle")}</DialogTitle>
            <DialogDescription>{agentName}</DialogDescription>
          </div>
          <div className={agentLogsDialogStyles["agent-logs-header-actions"]}>
            <Tooltip content={t("refreshLogs")}>
              <span>
                <IconButton
                  icon={<RefreshCw size={18} strokeWidth={2} />}
                  iconSize={16}
                  markClassName="icon-button-mark"
                  className="icon-button agent-logs-refresh"
                  label={t("refreshLogs")}
                  loading={loading}
                  loadingLabel={t("agentLogsLoading")}
                  onClick={onRefresh}
                />
              </span>
            </Tooltip>
            <DialogCloseButton className="icon-button" label={t("close")} />
          </div>
        </DialogHeader>
        <DialogBody className={agentLogsDialogStyles["agent-logs-body"]}>
          {error ? <InlineError className={`${agentLogsDialogStyles["agent-logs-error"]}`}>{error}</InlineError> : null}
          <pre ref={logsViewerRef} className={agentLogsDialogStyles["agent-logs-viewer"]}>
            {displayContent}
          </pre>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

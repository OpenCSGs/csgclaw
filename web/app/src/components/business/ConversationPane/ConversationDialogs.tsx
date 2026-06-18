import { useLayoutEffect, useRef } from "react";
import { RefreshCw } from "lucide-react";
import {
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { VoidOrPromise } from "./types";

export type ConversationRoomDangerConfirmDialogProps = {
  cancelLabel: string;
  closeLabel: string;
  confirmLabel: string;
  description: string;
  open: boolean;
  title: string;
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
};

export function ConversationRoomDangerConfirmDialog({
  cancelLabel,
  closeLabel,
  confirmLabel,
  description,
  open,
  title,
  onConfirm,
  onOpenChange,
}: ConversationRoomDangerConfirmDialogProps) {
  return (
    <DialogRoot open={open} onOpenChange={onOpenChange}>
      <DialogContent className="room-danger-dialog" overlayClassName="room-danger-backdrop">
        <DialogHeader className="room-danger-header">
          <div className="room-danger-copy">
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </div>
          <DialogCloseButton className="room-danger-close" label={closeLabel} size="sm" variant="tertiaryGray" />
        </DialogHeader>
        <div className="room-danger-actions">
          <Button className="room-danger-button" size="sm" variant="secondaryGray" onClick={() => onOpenChange(false)}>
            {cancelLabel}
          </Button>
          <Button className="room-danger-button" size="sm" variant="danger" onClick={onConfirm}>
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
  const displayContent = content || (loading ? t("agentLogsLoading") : t("agentLogsEmpty"));

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
      <DialogContent className="agent-logs-modal" overlayClassName="agent-logs-backdrop">
        <DialogHeader className="agent-logs-header">
          <div>
            <DialogTitle>{t("agentLogsTitle")}</DialogTitle>
            <DialogDescription>{agentName}</DialogDescription>
          </div>
          <div className="agent-logs-header-actions">
            <Button
              className="icon-button agent-logs-refresh"
              aria-label={t("refreshLogs")}
              title={t("refreshLogs")}
              loading={loading}
              loadingLabel={t("agentLogsLoading")}
              onClick={onRefresh}
            >
              <span className="icon-button-mark" aria-hidden="true">
                <RefreshCw size={18} strokeWidth={2} />
              </span>
            </Button>
            <DialogCloseButton className="icon-button" label={t("close")} />
          </div>
        </DialogHeader>
        <DialogBody className="agent-logs-body">
          {error ? <div className="form-error agent-logs-error">{error}</div> : null}
          <pre ref={logsViewerRef} className="agent-logs-viewer">
            {displayContent}
          </pre>
        </DialogBody>
      </DialogContent>
    </DialogRoot>
  );
}

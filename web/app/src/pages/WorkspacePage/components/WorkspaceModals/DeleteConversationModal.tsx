import { Button } from "@/components/ui";
import { ModalCloseButton } from "./ModalCloseButton";

export function DeleteConversationModal({
  busy = false,
  conversationTitle,
  error = "",
  isDirect,
  onCancel,
  onConfirm,
  t,
}) {
  const titleID = "delete-conversation-title";
  const descriptionID = "delete-conversation-description";

  return (
    <div className="modal-backdrop delete-conversation-backdrop">
      <div
        className="modal-card delete-conversation-modal"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleID}
        aria-describedby={descriptionID}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="modal-header">
          <div>
            <div id={titleID} className="modal-title">
              {isDirect ? t("deleteDirectMessageConfirmTitle") : t("deleteRoomConfirmTitle")}
            </div>
            <div id={descriptionID} className="modal-subtitle">
              {isDirect
                ? t("deleteDirectMessageConfirmDescription", { name: conversationTitle })
                : t("deleteRoomConfirmDescription", { name: conversationTitle })}
            </div>
          </div>
          <ModalCloseButton label={t("close")} disabled={busy} onClose={onCancel} />
        </div>
        {error ? <div className="form-error delete-conversation-error">{error}</div> : null}
        <div className="modal-actions">
          <Button variant="secondaryGray" size="lg" disabled={busy} onClick={onCancel}>
            {t("cancel")}
          </Button>
          <Button variant="danger" size="lg" loading={busy} loadingLabel={t("deleting")} onClick={onConfirm}>
            {t("confirmDelete")}
          </Button>
        </div>
      </div>
    </div>
  );
}

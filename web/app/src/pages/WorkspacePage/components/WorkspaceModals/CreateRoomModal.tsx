import { isBlank, requiredFieldLabel } from "@/components/business/ProfileControls";
import { Button } from "@/components/ui";
import { toggleSelection } from "@/shared/lib/collections";

export function CreateRoomModal({
  t,
  roomTitle,
  onRoomTitleChange,
  roomDescription,
  onRoomDescriptionChange,
  candidates,
  roomMemberIDs,
  lockedRoomMemberIDs,
  onRoomMemberIDsChange,
  submitError,
  onClose,
  onCreate,
}) {
  const candidateIDs = candidates.map((user) => user.id).filter(Boolean);
  const selectableMemberIDs = candidateIDs.filter((id) => !lockedRoomMemberIDs.includes(id));
  const allMembersSelected = candidateIDs.length > 0 && candidateIDs.every((id) => roomMemberIDs.includes(id));
  const selectedMemberCount = candidateIDs.filter((id) => roomMemberIDs.includes(id)).length;

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("createRoomTitle")}</div>
            <div className="modal-subtitle">{t("createRoomSubtitle")}</div>
          </div>
          <Button className="modal-close" onClick={onClose}>
            {t("close")}
          </Button>
        </div>
        <label className="field">
          {requiredFieldLabel(t("roomName"))}
          <input
            value={roomTitle}
            required
            aria-required="true"
            onInput={(event) => onRoomTitleChange(event.currentTarget.value)}
            placeholder={t("roomNamePlaceholder")}
          />
        </label>
        <label className="field">
          <span>{t("roomDescription")}</span>
          <textarea
            value={roomDescription}
            onInput={(event) => onRoomDescriptionChange(event.currentTarget.value)}
            placeholder={t("roomDescriptionPlaceholder")}
          />
        </label>
        <div className="field">
          <span>{t("initialMembers")}</span>
          <div className="selection-list">
            <label className="selection-item selection-all-item">
              <input
                type="checkbox"
                checked={allMembersSelected}
                disabled={selectableMemberIDs.length === 0}
                onChange={() => {
                  onRoomMemberIDsChange((current) => {
                    const allSelected = candidateIDs.length > 0 && candidateIDs.every((id) => current.includes(id));
                    if (allSelected) {
                      return current.filter((id) => !selectableMemberIDs.includes(id));
                    }
                    return Array.from(new Set([...current, ...selectableMemberIDs]));
                  });
                }}
              />
              <span>{t("allMembers")}</span>
              <small>
                {selectedMemberCount}/{candidateIDs.length}
              </small>
            </label>
            {candidates.map((user) => (
              <label key={user.id} className="selection-item">
                <input
                  type="checkbox"
                  checked={roomMemberIDs.includes(user.id)}
                  disabled={lockedRoomMemberIDs.includes(user.id)}
                  onChange={() => onRoomMemberIDsChange((current) => toggleSelection(current, user.id))}
                />
                <span>{user.name}</span>
                <small>@{user.handle}</small>
              </label>
            ))}
          </div>
        </div>
        {submitError ? <div className="form-error">{submitError}</div> : null}
        <div className="modal-actions">
          <Button className="secondary-button" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" className="send-button" disabled={isBlank(roomTitle)} onClick={onCreate}>
            {t("create")}
          </Button>
        </div>
      </div>
    </div>
  );
}

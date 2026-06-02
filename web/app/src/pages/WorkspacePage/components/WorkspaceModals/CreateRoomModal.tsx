import { isBlank } from "@/components/business/ProfileControls";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { Button as CSGButton } from "@/components/ui/Button";
import { useEffect } from "react";
import { toggleSelection } from "@/shared/lib/collections";
import { ModalCloseButton } from "./ModalCloseButton";

function getAvatarInitial(user) {
  const label = user.name || user.handle || user.id || "?";
  return label.trim().charAt(0).toUpperCase() || "?";
}

function MemberAvatar({ user, index = 0, compact = false }) {
  const fallback = getAvatarInitial(user);
  return (
    <span className={compact ? "create-room-avatar compact" : "create-room-avatar"} data-avatar-index={index % 6}>
      <AgentAvatarContent avatar={user.avatar} fallback={fallback} />
    </span>
  );
}

function AvatarStack({ users }) {
  const visibleUsers = users.slice(0, 9);
  const overflowCount = Math.max(users.length - visibleUsers.length, 0);

  return (
    <span className="create-room-avatar-stack" aria-hidden="true">
      {visibleUsers.map((user, index) => (
        <MemberAvatar key={user.id || index} user={user} index={index} compact />
      ))}
      {overflowCount > 0 ? <span className="create-room-avatar-more">+{overflowCount}</span> : null}
    </span>
  );
}

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

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
      }
    }

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose]);

  return (
    <div className="modal-backdrop">
      <div className="modal-card create-room-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("createRoomTitle")}</div>
            <div className="modal-subtitle">{t("createRoomSubtitle")}</div>
          </div>
          <ModalCloseButton label={t("close")} onClose={onClose} />
        </div>
        <div className="create-room-modal-content">
          <div className="create-room-section">
            <div className="create-room-section-title">{t("basicInfo")}</div>
            <label className="field create-room-field">
              <span className="field-label">
                <span className="field-required-star" aria-hidden="true">
                  *
                </span>
                {t("roomName")}
              </span>
              <input
                value={roomTitle}
                required
                aria-required="true"
                onInput={(event) => onRoomTitleChange(event.currentTarget.value)}
                placeholder={t("roomNamePlaceholder")}
              />
            </label>
            <label className="field create-room-field">
              <span>{t("roomDescription")}</span>
              <textarea
                value={roomDescription}
                onInput={(event) => onRoomDescriptionChange(event.currentTarget.value)}
                placeholder={t("roomDescriptionPlaceholder")}
              />
            </label>
          </div>

          <div className="create-room-section">
            <div className="create-room-section-title">{t("initialMembers")}</div>
            <div className="selection-list create-room-member-list">
              <label className="selection-item create-room-member-row">
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
                <span className="create-room-member-copy">
                  <strong>{t("allMembers")}</strong>
                  <small>
                    {selectedMemberCount}/{candidateIDs.length}
                  </small>
                </span>
                <AvatarStack users={candidates} />
              </label>
              {candidates.map((user, index) => (
                <label key={user.id} className="selection-item create-room-member-row">
                  <input
                    type="checkbox"
                    checked={roomMemberIDs.includes(user.id)}
                    disabled={lockedRoomMemberIDs.includes(user.id)}
                    onChange={() => onRoomMemberIDsChange((current) => toggleSelection(current, user.id))}
                  />
                  <MemberAvatar user={user} index={index} />
                  <span className="create-room-member-copy">
                    <strong>{user.name}</strong>
                    <small>@{user.handle}</small>
                  </span>
                </label>
              ))}
            </div>
          </div>
          {submitError ? <div className="form-error create-room-error">{submitError}</div> : null}
        </div>
        <div className="modal-actions">
          <CSGButton variant="secondaryGray" size="md" onClick={onClose}>
            {t("cancel")}
          </CSGButton>
          <CSGButton variant="primary" size="md" disabled={isBlank(roomTitle)} onClick={onCreate}>
            {t("create")}
          </CSGButton>
        </div>
      </div>
    </div>
  );
}

import { Button } from "@/components/ui";
import { toggleSelection } from "@/shared/lib/collections";
import { ModalCloseButton } from "./ModalCloseButton";

function getAvatarInitial(user) {
  if (user?.avatar && typeof user.avatar === "string" && user.avatar.length <= 2) {
    return user.avatar;
  }
  return (user?.name || user?.handle || "?").charAt(0).toUpperCase();
}

function MemberAvatar({ user, index = 0, compact = false }) {
  return (
    <span className={compact ? "create-room-avatar compact" : "create-room-avatar"} data-avatar-index={index % 6}>
      {getAvatarInitial(user)}
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

export function InviteMembersModal({
  t,
  candidates,
  inviteUserIDs,
  onInviteUserIDsChange,
  submitError,
  onClose,
  onInvite,
}) {
  const candidateIDs = candidates.map((user) => user.id).filter(Boolean);
  const allCandidatesSelected = candidateIDs.length > 0 && candidateIDs.every((id) => inviteUserIDs.includes(id));
  const selectedMemberCount = candidateIDs.filter((id) => inviteUserIDs.includes(id)).length;

  return (
    <div className="modal-backdrop">
      <div className="modal-card invite-members-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("inviteTitle")}</div>
            <div className="modal-subtitle">{t("inviteSubtitle")}</div>
          </div>
          <ModalCloseButton label={t("close")} onClose={onClose} />
        </div>
        <div className="invite-members-content">
          <div className="field">
            <span>{t("inviteCandidates")}</span>
            <div className="selection-list create-room-member-list">
              {candidates.length > 0 ? (
                <>
                  <label className="selection-item create-room-member-row">
                    <input
                      type="checkbox"
                      checked={allCandidatesSelected}
                      onChange={() => {
                        onInviteUserIDsChange((current) => {
                          const allSelected = candidateIDs.length > 0 && candidateIDs.every((id) => current.includes(id));
                          if (allSelected) {
                            return current.filter((id) => !candidateIDs.includes(id));
                          }
                          return Array.from(new Set([...current, ...candidateIDs]));
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
                        checked={inviteUserIDs.includes(user.id)}
                        onChange={() => onInviteUserIDsChange((current) => toggleSelection(current, user.id))}
                      />
                      <MemberAvatar user={user} index={index} />
                      <span className="create-room-member-copy">
                        <strong>{user.name}</strong>
                        <small>@{user.handle}</small>
                      </span>
                    </label>
                  ))}
                </>
              ) : (
                <div className="selection-empty">{t("noInviteCandidates")}</div>
              )}
            </div>
          </div>
          {submitError ? <div className="form-error">{submitError}</div> : null}
        </div>
        <div className="modal-actions">
          <Button variant="secondaryGray" size="md" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" size="md" disabled={inviteUserIDs.length === 0} onClick={onInvite}>
            {t("sendInvite")}
          </Button>
        </div>
      </div>
    </div>
  );
}

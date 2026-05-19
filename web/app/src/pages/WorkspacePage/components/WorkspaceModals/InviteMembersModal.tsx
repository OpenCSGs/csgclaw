// @ts-nocheck
import { Button } from "@/components/ui";
import { toggleSelection } from "@/shared/lib/collections";

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
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("inviteTitle")}</div>
            <div className="modal-subtitle">{t("inviteSubtitle")}</div>
          </div>
          <Button className="modal-close" onClick={onClose}>
            {t("close")}
          </Button>
        </div>
        <div className="field">
          <span>{t("inviteCandidates")}</span>
          <div className="selection-list">
            {candidates.length > 0 ? (
              <>
                <label className="selection-item selection-all-item">
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
                  <span>{t("allMembers")}</span>
                  <small>
                    {selectedMemberCount}/{candidateIDs.length}
                  </small>
                </label>
                {candidates.map((user) => (
                  <label key={user.id} className="selection-item">
                    <input
                      type="checkbox"
                      checked={inviteUserIDs.includes(user.id)}
                      onChange={() => onInviteUserIDsChange((current) => toggleSelection(current, user.id))}
                    />
                    <span>{user.name}</span>
                    <small>@{user.handle}</small>
                  </label>
                ))}
              </>
            ) : (
              <div className="selection-empty">{t("noInviteCandidates")}</div>
            )}
          </div>
        </div>
        {submitError ? <div className="form-error">{submitError}</div> : null}
        <div className="modal-actions">
          <Button className="secondary-button" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" className="send-button" disabled={inviteUserIDs.length === 0} onClick={onInvite}>
            {t("sendInvite")}
          </Button>
        </div>
      </div>
    </div>
  );
}

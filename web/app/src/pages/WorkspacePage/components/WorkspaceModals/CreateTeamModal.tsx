import { Button } from "@/components/ui";
import { toggleSelection } from "@/shared/lib/collections";
import type { AgentLike } from "@/models/agents";
import { requiredFieldLabel } from "@/components/business/ProfileControls";
import { ModalCloseButton } from "./ModalCloseButton";
import type { TranslateFn } from "@/models/conversations";
import type { Dispatch, SetStateAction } from "react";

type CreateTeamModalProps = {
  t: TranslateFn;
  candidates: AgentLike[];
  teamTitle: string;
  onTeamTitleChange: (value: string) => void;
  teamMemberIDs: string[];
  onTeamMemberIDsChange: Dispatch<SetStateAction<string[]>>;
  submitError: string;
  teamActionBusy: boolean;
  onClose: () => void;
  onCreate: () => Promise<void>;
};

export function CreateTeamModal({
  t,
  candidates,
  teamTitle,
  onTeamTitleChange,
  teamMemberIDs,
  onTeamMemberIDsChange,
  submitError,
  teamActionBusy,
  onClose,
  onCreate,
}: CreateTeamModalProps) {
  const candidateIDs = candidates.map((item) => item.id).filter((id): id is string => Boolean(id));
  const allCandidatesSelected = candidateIDs.length > 0 && candidateIDs.every((id) => teamMemberIDs.includes(id));
  const selectedMemberCount = candidateIDs.filter((id) => teamMemberIDs.includes(id)).length;

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("teamCreate")}</div>
            <div className="modal-subtitle">{t("teamMembersSubtitle")}</div>
          </div>
          <ModalCloseButton label={t("close")} onClose={onClose} />
        </div>

        <label className="field">
          <span>{requiredFieldLabel(t("teamNameLabel"))}</span>
          <input
            value={teamTitle}
            onInput={(event) => onTeamTitleChange(event.currentTarget.value)}
            placeholder={t("teamNamePlaceholder")}
          />
        </label>

        <div className="field">
          <span>{t("teamMembersLabel")}</span>
          <div className="selection-list">
            {candidates.length ? (
              <>
                <label className="selection-item selection-all-item">
                  <input
                    type="checkbox"
                    checked={allCandidatesSelected}
                    onChange={() =>
                      onTeamMemberIDsChange((current) =>
                        allCandidatesSelected
                          ? current.filter((id) => !candidateIDs.includes(id))
                          : Array.from(new Set([...current, ...candidateIDs])),
                      )
                    }
                  />
                  <span>{t("allMembers")}</span>
                  <small>
                    {selectedMemberCount}/{candidateIDs.length}
                  </small>
                </label>
                {candidates.map((item) => {
                  const itemID = item.id || "";
                  return (
                    <label key={itemID || item.name} className="selection-item">
                      <input
                        type="checkbox"
                        checked={itemID ? teamMemberIDs.includes(itemID) : false}
                        disabled={!itemID}
                        onChange={() =>
                          itemID ? onTeamMemberIDsChange((current) => toggleSelection(current, itemID)) : undefined
                        }
                      />
                      <span>{item.name || itemID}</span>
                      <small>{item.role || "-"}</small>
                    </label>
                  );
                })}
              </>
            ) : (
              <div className="selection-empty">{t("teamNoMembersHint")}</div>
            )}
          </div>
        </div>

        {submitError ? <div className="form-error">{submitError}</div> : null}

        <div className="modal-actions">
          <Button variant="secondaryGray" size="md" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            size="md"
            loading={teamActionBusy}
            loadingLabel={t("teamSaving")}
            disabled={teamActionBusy || teamMemberIDs.length === 0}
            onClick={onCreate}
          >
            {t("teamCreate")}
          </Button>
        </div>
      </div>
    </div>
  );
}

import { Button, Checkbox } from "@/components/ui";
import { toggleSelection } from "@/shared/lib/collections";
import type { AgentLike } from "@/models/agents";
import { requiredFieldLabel } from "@/components/business/ProfileControls";
import { ModalCloseButton } from "./ModalCloseButton";
import type { TranslateFn } from "@/models/conversations";
import type { Dispatch, SetStateAction } from "react";

type CreateTeamModalProps = {
  t: TranslateFn;
  candidates: AgentLike[];
  mode?: "create" | "edit";
  lockedTeamMemberIDs?: string[];
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
  mode = "create",
  lockedTeamMemberIDs = [],
  teamTitle,
  onTeamTitleChange,
  teamMemberIDs,
  onTeamMemberIDsChange,
  submitError,
  teamActionBusy,
  onClose,
  onCreate,
}: CreateTeamModalProps) {
  const editing = mode === "edit";
  const candidateIDs = candidates.map((item) => item.id).filter((id): id is string => Boolean(id));
  const selectableCandidateIDs = candidateIDs.filter((id) => !lockedTeamMemberIDs.includes(id));
  const isMemberSelected = (id: string) => lockedTeamMemberIDs.includes(id) || teamMemberIDs.includes(id);
  const allCandidatesSelected = candidateIDs.length > 0 && candidateIDs.every(isMemberSelected);
  const selectedMemberCount = candidateIDs.filter(isMemberSelected).length;

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{editing ? t("teamManageMembers") : t("teamCreate")}</div>
            <div className="modal-subtitle">{editing ? t("teamManageMembersSubtitle") : t("teamMembersSubtitle")}</div>
          </div>
          <ModalCloseButton label={t("close")} onClose={onClose} />
        </div>

        {editing ? null : (
          <label className="field">
            <span>{requiredFieldLabel(t("teamNameLabel"))}</span>
            <input
              autoFocus
              value={teamTitle}
              onChange={(event) => onTeamTitleChange(event.currentTarget.value)}
              placeholder={t("teamNamePlaceholder")}
            />
          </label>
        )}

        <div className="field">
          <span>{t("teamMembersLabel")}</span>
          <div className={`selection-list${editing ? " team-members-dialog-list" : ""}`}>
            {candidates.length ? (
              <>
                <label className="selection-item selection-all-item" htmlFor="team-member-all">
                  <Checkbox
                    id="team-member-all"
                    checked={allCandidatesSelected}
                    disabled={selectableCandidateIDs.length === 0}
                    onCheckedChange={() => {
                      onTeamMemberIDsChange((current) => {
                        const allSelected = candidateIDs.every(
                          (id) => lockedTeamMemberIDs.includes(id) || current.includes(id),
                        );
                        return allSelected
                          ? current.filter((id) => !selectableCandidateIDs.includes(id))
                          : Array.from(new Set([...current, ...selectableCandidateIDs]));
                      });
                    }}
                  />
                  <span>{t("allMembers")}</span>
                  <small>
                    {selectedMemberCount}/{candidateIDs.length}
                  </small>
                </label>
                {candidates.map((item) => {
                  const itemID = item.id || "";
                  const memberLocked = itemID ? lockedTeamMemberIDs.includes(itemID) : false;
                  const checkboxID = teamMemberCheckboxID(itemID || item.name);
                  return (
                    <label key={itemID || item.name} className="selection-item" htmlFor={checkboxID}>
                      <Checkbox
                        id={checkboxID}
                        checked={itemID ? memberLocked || teamMemberIDs.includes(itemID) : false}
                        disabled={!itemID || memberLocked}
                        onCheckedChange={() =>
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
            disabled={teamActionBusy || (!editing && teamMemberIDs.length === 0)}
            onClick={onCreate}
          >
            {editing ? t("teamSaveMembers") : t("teamCreate")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function teamMemberCheckboxID(value: string | null | undefined): string {
  return `team-member-${String(value || "unknown").replace(/[^a-zA-Z0-9_-]+/g, "-")}`;
}

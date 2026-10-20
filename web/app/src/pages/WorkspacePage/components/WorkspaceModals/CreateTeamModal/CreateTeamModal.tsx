import {
  Button,
  Checkbox,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogCloseButton,
  DialogBody,
  DialogFooter,
  Field,
  TextInput,
} from "@/components/ui";
import { useRef } from "react";
import styles from "./CreateTeamModal.module.css";
import { toggleSelection } from "@/shared/lib/collections";
import type { AgentLike } from "@/models/agents";
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
  const nameInputRef = useRef<HTMLInputElement>(null);
  const editing = mode === "edit";
  const candidateIDs = candidates.map((item) => item.id).filter((id): id is string => Boolean(id));
  const selectableCandidateIDs = candidateIDs.filter((id) => !lockedTeamMemberIDs.includes(id));
  const isMemberSelected = (id: string) => lockedTeamMemberIDs.includes(id) || teamMemberIDs.includes(id);
  const allCandidatesSelected = candidateIDs.length > 0 && candidateIDs.every(isMemberSelected);
  const selectedMemberCount = candidateIDs.filter(isMemberSelected).length;

  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open && !teamActionBusy) onClose();
      }}
    >
      <DialogContent
        aria-modal="true"
        className={styles.dialog}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          nameInputRef.current?.focus();
        }}
      >
        <DialogHeader className={styles.header}>
          <div>
            <DialogTitle>{editing ? t("teamManageMembers") : t("teamCreate")}</DialogTitle>
            <DialogDescription>{editing ? t("teamManageMembersSubtitle") : t("teamMembersSubtitle")}</DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} variant="tertiaryGray" size="sm" disabled={teamActionBusy} />
        </DialogHeader>
        <DialogBody className={styles.body}>
          <Field label={t("teamNameLabel")} required>
            <TextInput
              ref={nameInputRef}
              value={teamTitle}
              disabled={teamActionBusy}
              onChange={(event) => onTeamTitleChange(event.currentTarget.value)}
              placeholder={t("teamNamePlaceholder")}
            />
          </Field>

          <div className="field">
            <span>{t("teamMembersLabel")}</span>
            <div className="selection-list">
              {candidates.length ? (
                <>
                  <label className="selection-item selection-all-item" htmlFor="team-member-all">
                    <Checkbox
                      id="team-member-all"
                      checked={allCandidatesSelected}
                      disabled={teamActionBusy || selectableCandidateIDs.length === 0}
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
                          disabled={teamActionBusy || !itemID || memberLocked}
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

          {submitError ? (
            <div className="form-error" role="alert">
              {submitError}
            </div>
          ) : null}
        </DialogBody>

        <DialogFooter className={styles.footer}>
          <Button variant="secondaryGray" size="md" onClick={onClose} disabled={teamActionBusy}>
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
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

function teamMemberCheckboxID(value: string | null | undefined): string {
  return `team-member-${String(value || "unknown").replace(/[^a-zA-Z0-9_-]+/g, "-")}`;
}

import type { Dispatch, SetStateAction } from "react";
import {
  Button,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Select,
} from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import { TaskDialogCloseButton } from "./TaskDialogCloseButton";
import { TASK_TITLE_MAX_LENGTH } from "./taskDialogTypes";
import styles from "./TasksView.module.css";
import type { SelectOption } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { TaskCreateDraft, TaskCreateFieldErrors } from "./taskDialogTypes";

type TaskCreateDialogProps = {
  assignmentOptions: readonly SelectOption[];
  busy: boolean;
  draft: TaskCreateDraft;
  error: string;
  errors: TaskCreateFieldErrors;
  onChange: Dispatch<SetStateAction<TaskCreateDraft>>;
  onClearError: (field: keyof TaskCreateFieldErrors) => void;
  onClose?: () => void;
  onSubmit: () => void | Promise<void>;
  open: boolean;
  t: TranslateFn;
};

export function TaskCreateDialog({
  assignmentOptions,
  busy,
  draft,
  error,
  errors,
  open,
  t,
  onChange,
  onClearError,
  onClose,
  onSubmit,
}: TaskCreateDialogProps) {
  return (
    <DialogRoot open={open} onOpenChange={(nextOpen) => (!nextOpen ? onClose?.() : null)}>
      <DialogContent className={styles.taskCreateDialog}>
        <DialogHeader>
          <div>
            <DialogTitle>{t("taskCreateTitle")}</DialogTitle>
            <DialogDescription>{t("taskCreateSubtitle")}</DialogDescription>
          </div>
          <TaskDialogCloseButton label={t("close")} />
        </DialogHeader>
        <DialogBody>
          <div className={classNames(styles.taskCreateForm, styles.taskCreateFormCompact)}>
            <label
              className={classNames("field", styles.taskCreateField)}
              data-invalid={errors.title ? true : undefined}
            >
              <span>{t("taskTitleLabel")}</span>
              <input
                value={draft.title}
                maxLength={TASK_TITLE_MAX_LENGTH}
                aria-describedby={errors.title ? "task-create-title-error" : undefined}
                aria-invalid={errors.title ? true : undefined}
                onInput={(event) => {
                  const value = event.currentTarget.value;
                  onChange((current) => ({ ...current, title: value }));
                  onClearError("title");
                }}
                placeholder={t("taskTitlePlaceholder")}
              />
              {errors.title ? (
                <span id="task-create-title-error" className="form-error" role="alert">
                  {errors.title}
                </span>
              ) : null}
            </label>
            <label className={classNames("field", styles.taskCreateField)}>
              <span>{t("taskDescriptionLabel")}</span>
              <textarea
                value={draft.description}
                aria-label={t("taskDescriptionLabel")}
                onInput={(event) => {
                  const value = event.currentTarget.value;
                  onChange((current) => ({ ...current, description: value }));
                }}
                placeholder={t("taskDescriptionPlaceholder")}
              />
            </label>
            <label
              className={classNames("field", styles.taskCreateField)}
              data-invalid={errors.assignment ? true : undefined}
            >
              <span>{t("taskAssignmentLabel")}</span>
              <Select
                value={draft.assignee}
                onValueChange={(assignee) => {
                  onChange((current) => ({ ...current, assignee }));
                  onClearError("assignment");
                }}
                triggerProps={{
                  "aria-describedby": errors.assignment ? "task-create-assignment-error" : undefined,
                  "aria-invalid": errors.assignment ? true : undefined,
                  "aria-label": t("taskAssignmentLabel"),
                }}
                options={assignmentOptions}
                placeholder={t("taskAssignmentPlaceholder")}
              />
              {errors.assignment ? (
                <span id="task-create-assignment-error" className="form-error" role="alert">
                  {errors.assignment}
                </span>
              ) : null}
            </label>
          </div>
          {error ? <div className={classNames("form-error", styles.taskCreateError)}>{error}</div> : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="secondaryGray" size="md" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            size="md"
            loading={busy}
            loadingLabel={t("taskCreating")}
            disabled={busy}
            onClick={onSubmit}
          >
            {t("taskCreateSubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

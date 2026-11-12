import type { Dispatch, SetStateAction } from "react";
import {
  InlineError,
  Field,
  TextInput,
  TextArea,
  Button,
  DialogBody,
  DialogContent,
  DialogCloseButton,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Select,
} from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import { TASK_TITLE_MAX_LENGTH } from "./taskDialogTypes";
import taskFormStyles from "./TaskForm.module.css";
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
      <DialogContent className={taskFormStyles.taskCreateDialog}>
        <DialogHeader>
          <div>
            <DialogTitle>{t("taskCreateTitle")}</DialogTitle>
            <DialogDescription>{t("taskCreateSubtitle")}</DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
        </DialogHeader>
        <DialogBody>
          <div className={classNames(taskFormStyles.taskCreateForm, taskFormStyles.taskCreateFormCompact)}>
            <Field className={taskFormStyles.taskCreateField} label={t("taskTitleLabel")} error={errors.title}>
              <TextInput
                value={draft.title}
                maxLength={TASK_TITLE_MAX_LENGTH}
                onInput={(event) => {
                  const value = event.currentTarget.value;
                  onChange((current) => ({ ...current, title: value }));
                  onClearError("title");
                }}
                placeholder={t("taskTitlePlaceholder")}
              />
            </Field>
            <Field className={taskFormStyles.taskCreateField} label={t("taskDescriptionLabel")}>
              <TextArea
                value={draft.description}
                onInput={(event) => {
                  const value = event.currentTarget.value;
                  onChange((current) => ({ ...current, description: value }));
                }}
                placeholder={t("taskDescriptionPlaceholder")}
              />
            </Field>
            <Field
              className={taskFormStyles.taskCreateField}
              label={t("taskAssignmentLabel")}
              error={errors.assignment}
            >
              <Select
                value={draft.assignee}
                onValueChange={(assignee) => {
                  onChange((current) => ({ ...current, assignee }));
                  onClearError("assignment");
                }}
                options={assignmentOptions}
                placeholder={t("taskAssignmentPlaceholder")}
              />
            </Field>
          </div>
          {error ? <InlineError className={taskFormStyles.taskCreateError}>{error}</InlineError> : null}
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

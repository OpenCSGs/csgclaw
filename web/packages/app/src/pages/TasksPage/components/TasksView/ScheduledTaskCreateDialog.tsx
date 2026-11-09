import {
  Button,
  DialogBody,
  DialogContent,
  DialogCloseButton,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import { ScheduledTaskFormFields } from "./ScheduledTaskFormFields";
import taskFormStyles from "./TaskForm.module.css";
import type { Dispatch, SetStateAction } from "react";
import type { SelectOption } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { ScheduledTaskFormDraft, ScheduledTaskFormFieldErrors } from "./taskDialogTypes";

type ScheduledTaskCreateDialogProps = {
  busy: boolean;
  draft: ScheduledTaskFormDraft;
  error: string;
  errors: ScheduledTaskFormFieldErrors;
  onChange: Dispatch<SetStateAction<ScheduledTaskFormDraft>>;
  onClearError: (field: keyof ScheduledTaskFormFieldErrors) => void;
  onClose?: () => void;
  onSubmit: () => void | Promise<void>;
  open: boolean;
  scheduledAgentOptions: readonly SelectOption[];
  t: TranslateFn;
};

export function ScheduledTaskCreateDialog({
  busy,
  draft,
  error,
  errors,
  open,
  scheduledAgentOptions,
  t,
  onChange,
  onClearError,
  onClose,
  onSubmit,
}: ScheduledTaskCreateDialogProps) {
  return (
    <DialogRoot open={open} onOpenChange={(nextOpen) => (!nextOpen ? onClose?.() : null)}>
      <DialogContent className={taskFormStyles.taskCreateDialog}>
        <DialogHeader>
          <div>
            <DialogTitle>{t("scheduledTaskCreateTitle")}</DialogTitle>
            <DialogDescription>{t("scheduledTaskCreateSubtitle")}</DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
        </DialogHeader>
        <DialogBody>
          <ScheduledTaskFormFields
            draft={draft}
            errors={errors}
            scheduledAgentOptions={scheduledAgentOptions}
            t={t}
            onChange={onChange}
            onClearError={onClearError}
          />
          {error ? <div className={classNames("form-error", taskFormStyles.taskCreateError)}>{error}</div> : null}
        </DialogBody>
        <DialogFooter>
          <Button variant="secondaryGray" size="md" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            size="md"
            loading={busy}
            loadingLabel={t("scheduledTaskCreating")}
            disabled={busy}
            onClick={onSubmit}
          >
            {t("scheduledTaskCreateSubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

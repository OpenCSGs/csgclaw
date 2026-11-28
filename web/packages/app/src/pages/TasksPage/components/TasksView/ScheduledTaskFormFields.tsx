import type { Dispatch, SetStateAction } from "react";
import { Field, TextInput, TextArea, Combobox } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import { TASK_TITLE_MAX_LENGTH } from "./taskDialogTypes";
import taskFormStyles from "./TaskForm.module.css";
import type { SelectOption } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { ScheduledTaskFormDraft, ScheduledTaskFormFieldErrors } from "./taskDialogTypes";
import { ScheduleControls } from "./ScheduleControls";

type ScheduledTaskFormFieldsProps = {
  draft: ScheduledTaskFormDraft;
  errors: ScheduledTaskFormFieldErrors;
  onChange: Dispatch<SetStateAction<ScheduledTaskFormDraft>>;
  onClearError: (field: keyof ScheduledTaskFormFieldErrors) => void;
  scheduledAgentOptions: readonly SelectOption[];
  t: TranslateFn;
};

export function ScheduledTaskFormFields({
  draft,
  errors,
  scheduledAgentOptions,
  t,
  onChange,
  onClearError,
}: ScheduledTaskFormFieldsProps) {
  return (
    <div className={taskFormStyles.taskCreateForm}>
      <Field className={taskFormStyles.taskCreateField} label={t("taskTitleLabel")} error={errors.title}>
        <TextInput
          value={draft.title}
          maxLength={TASK_TITLE_MAX_LENGTH}
          placeholder={t("taskTitlePlaceholder")}
          onInput={(event) => {
            const value = event.currentTarget.value;
            onChange((current) => ({ ...current, title: value }));
            onClearError("title");
          }}
        />
      </Field>
      <Field className={taskFormStyles.taskCreateField} label={t("scheduledTaskAgentLabel")} error={errors.agentID}>
        <Combobox
          value={draft.agentID}
          onValueChange={(agentID) => {
            onChange((current) => ({ ...current, agentID }));
            onClearError("agentID");
          }}
          options={scheduledAgentOptions}
          placeholder={t("scheduledTaskAgentPlaceholder")}
          triggerProps={{ "aria-label": t("scheduledTaskAgentLabel") }}
          searchPlaceholder={t("scheduledTaskAgentPlaceholder")}
          emptyLabel={t("scheduledTaskExecutorEmpty")}
        />
      </Field>
      <Field
        className={classNames(taskFormStyles.taskCreateField, taskFormStyles.span2)}
        label={t("scheduledTaskPromptLabel")}
        error={errors.prompt}
      >
        <TextArea
          value={draft.prompt}
          placeholder={t("scheduledTaskPromptPlaceholder")}
          onInput={(event) => {
            const value = event.currentTarget.value;
            onChange((current) => ({ ...current, prompt: value }));
            onClearError("prompt");
          }}
        />
      </Field>
      <div className={taskFormStyles.span2}>
        <ScheduleControls
          draft={draft}
          errors={errors}
          t={t}
          onChange={(patch) => {
            onChange((current) => ({ ...current, ...patch }));
            (["weekdays", "monthDays", "interval", "onceAt", "time", "effectiveRange"] as const).forEach(onClearError);
          }}
        />
      </div>
    </div>
  );
}

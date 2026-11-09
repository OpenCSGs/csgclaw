import type { Dispatch, SetStateAction } from "react";
import { Field, TextInput, TextArea, Combobox, Select } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import { TASK_TITLE_MAX_LENGTH } from "./taskDialogTypes";
import taskFormStyles from "./TaskForm.module.css";
import type { SelectOption } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import type { ScheduledTaskRecurrence } from "@/models/scheduledTasks";
import type { ScheduledTaskFormDraft, ScheduledTaskFormFieldErrors } from "./taskDialogTypes";

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
      <Field className={taskFormStyles.taskCreateField} label={t("scheduledTaskRecurrenceLabel")}>
        <Select
          value={draft.recurrence}
          onValueChange={(recurrence) =>
            onChange((current) => ({ ...current, recurrence: recurrence as ScheduledTaskRecurrence }))
          }
          options={[
            { value: "once", label: t("scheduledTaskRecurrenceOnce") },
            { value: "daily", label: t("scheduledTaskRecurrenceDaily") },
            { value: "weekly", label: t("scheduledTaskRecurrenceWeekly") },
            { value: "monthly", label: t("scheduledTaskRecurrenceMonthly") },
          ]}
          triggerProps={{ "aria-label": t("scheduledTaskRecurrenceLabel") }}
        />
      </Field>
      <Field className={taskFormStyles.taskCreateField} label={t("scheduledTaskDateLabel")} error={errors.date}>
        <TextInput
          type="date"
          value={draft.date}
          onInput={(event) => {
            const value = event.currentTarget.value;
            onChange((current) => ({ ...current, date: value }));
            onClearError("date");
          }}
        />
      </Field>
      <Field className={taskFormStyles.taskCreateField} label={t("scheduledTaskTimeLabel")} error={errors.time}>
        <TextInput
          type="time"
          value={draft.time}
          onInput={(event) => {
            const value = event.currentTarget.value;
            onChange((current) => ({ ...current, time: value }));
            onClearError("time");
          }}
        />
      </Field>
      <Field className={taskFormStyles.taskCreateField} label={t("scheduledTaskExpiresLabel")}>
        <TextInput
          type="date"
          value={draft.expiresDate}
          onInput={(event) => {
            const value = event.currentTarget.value;
            onChange((current) => ({ ...current, expiresDate: value }));
          }}
        />
      </Field>
    </div>
  );
}

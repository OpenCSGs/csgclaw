import type { Dispatch, SetStateAction } from "react";
import { Select } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import { TASK_TITLE_MAX_LENGTH } from "./taskDialogTypes";
import styles from "./TasksView.module.css";
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
    <div className={styles.taskCreateForm}>
      <label className={classNames("field", styles.taskCreateField)} data-invalid={errors.title ? true : undefined}>
        <span>{t("taskTitleLabel")}</span>
        <input
          value={draft.title}
          maxLength={TASK_TITLE_MAX_LENGTH}
          placeholder={t("taskTitlePlaceholder")}
          onInput={(event) => {
            onChange((current) => ({ ...current, title: event.currentTarget.value }));
            onClearError("title");
          }}
        />
        {errors.title ? <span className="form-error">{errors.title}</span> : null}
      </label>
      <label className={classNames("field", styles.taskCreateField)} data-invalid={errors.agentID ? true : undefined}>
        <span>{t("scheduledTaskAgentLabel")}</span>
        <Select
          value={draft.agentID}
          onValueChange={(agentID) => {
            onChange((current) => ({ ...current, agentID }));
            onClearError("agentID");
          }}
          options={scheduledAgentOptions}
          placeholder={t("scheduledTaskAgentPlaceholder")}
          triggerProps={{ "aria-label": t("scheduledTaskAgentLabel") }}
        />
        {errors.agentID ? <span className="form-error">{errors.agentID}</span> : null}
      </label>
      <label
        className={classNames("field", styles.taskCreateField, styles.span2)}
        data-invalid={errors.prompt ? true : undefined}
      >
        <span>{t("scheduledTaskPromptLabel")}</span>
        <textarea
          value={draft.prompt}
          placeholder={t("scheduledTaskPromptPlaceholder")}
          onInput={(event) => {
            onChange((current) => ({ ...current, prompt: event.currentTarget.value }));
            onClearError("prompt");
          }}
        />
        {errors.prompt ? <span className="form-error">{errors.prompt}</span> : null}
      </label>
      <label className={classNames("field", styles.taskCreateField)}>
        <span>{t("scheduledTaskRecurrenceLabel")}</span>
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
      </label>
      <label className={classNames("field", styles.taskCreateField)} data-invalid={errors.date ? true : undefined}>
        <span>{t("scheduledTaskDateLabel")}</span>
        <input
          type="date"
          value={draft.date}
          onInput={(event) => {
            onChange((current) => ({ ...current, date: event.currentTarget.value }));
            onClearError("date");
          }}
        />
        {errors.date ? <span className="form-error">{errors.date}</span> : null}
      </label>
      <label className={classNames("field", styles.taskCreateField)} data-invalid={errors.time ? true : undefined}>
        <span>{t("scheduledTaskTimeLabel")}</span>
        <input
          type="time"
          value={draft.time}
          onInput={(event) => {
            onChange((current) => ({ ...current, time: event.currentTarget.value }));
            onClearError("time");
          }}
        />
        {errors.time ? <span className="form-error">{errors.time}</span> : null}
      </label>
      <label className={classNames("field", styles.taskCreateField)}>
        <span>{t("scheduledTaskExpiresLabel")}</span>
        <input
          type="date"
          value={draft.expiresDate}
          onInput={(event) => onChange((current) => ({ ...current, expiresDate: event.currentTarget.value }))}
        />
      </label>
    </div>
  );
}

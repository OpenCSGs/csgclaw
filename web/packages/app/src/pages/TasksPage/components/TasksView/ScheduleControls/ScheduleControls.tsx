import { useId } from "react";
import type { TranslateFn } from "@/models/conversations";
import {
  MAX_SCHEDULED_TASK_INTERVAL_MINUTES,
  SCHEDULED_TASK_INTERVAL_UNIT_MINUTES,
  type ScheduledTaskIntervalUnit,
  type ScheduledTaskRecurrence,
} from "@/models/scheduledTasks";
import { EffectiveRangeControl } from "./EffectiveRangeControl";
import { MomentControl } from "./MomentControl";
import { RecurrencePicker } from "./RecurrencePicker";
import { draftScheduleSummary, recurrencePatch } from "./scheduleDraft";
import type { ScheduledTaskFormDraft, ScheduledTaskFormFieldErrors } from "../taskDialogTypes";
import styles from "./ScheduleControls.module.css";

type ScheduleControlsProps = {
  draft: ScheduledTaskFormDraft;
  errors: ScheduledTaskFormFieldErrors;
  t: TranslateFn;

  onChange: (patch: Partial<ScheduledTaskFormDraft>) => void;
};

const INTERVAL_UNITS: ScheduledTaskIntervalUnit[] = ["minute", "hour", "day"];

export function ScheduleControls({ draft, errors, t, onChange }: ScheduleControlsProps) {
  const intervalID = useId();
  const error =
    errors.weekdays || errors.monthDays || errors.interval || errors.onceAt || errors.time || errors.effectiveRange;

  function changeType(next: ScheduledTaskRecurrence) {
    onChange({ recurrence: next, ...recurrencePatch(next, draft) });
  }

  function toggleWeekday(weekday: number) {
    const exists = draft.weekdays.includes(weekday);
    onChange({ weekdays: exists ? draft.weekdays.filter((item) => item !== weekday) : [...draft.weekdays, weekday] });
  }

  function toggleMonthDay(day: number) {
    const exists = draft.monthDays.includes(day);
    onChange({ monthDays: exists ? draft.monthDays.filter((item) => item !== day) : [...draft.monthDays, day] });
  }

  const momentMode = draft.recurrence === "once" ? "datetime" : draft.recurrence === "yearly" ? "yearly" : "time";

  const showEffectiveRange = draft.recurrence !== "once";

  return (
    <div className={styles.schedule}>
      <div className={`${styles.scheduleRow} ${showEffectiveRange ? "" : styles.scheduleRowCompact}`}>
        <RecurrencePicker
          value={draft.recurrence}
          summary={draftScheduleSummary(draft, t)}
          weekdays={draft.weekdays}
          monthDays={draft.monthDays}
          invalid={Boolean(errors.weekdays || errors.monthDays)}
          t={t}
          onChangeType={changeType}
          onToggleWeekday={toggleWeekday}
          onToggleMonthDay={toggleMonthDay}
        />
        {draft.recurrence === "interval" ? (
          <div className={styles.intervalControl} data-invalid={errors.interval ? "true" : undefined}>
            <label className={styles.intervalPrefix} htmlFor={intervalID}>
              {t("scheduledTaskIntervalLabel")}
            </label>
            <input
              id={intervalID}
              className={styles.intervalInput}
              type="number"
              min={1}
              max={MAX_SCHEDULED_TASK_INTERVAL_MINUTES / SCHEDULED_TASK_INTERVAL_UNIT_MINUTES[draft.intervalUnit]}
              inputMode="numeric"
              aria-invalid={Boolean(errors.interval)}
              value={draft.intervalValue || ""}
              onChange={(event) => {
                onChange({ intervalValue: Number(event.currentTarget.value) });
              }}
            />
            <select
              className={styles.intervalUnit}
              aria-label={t("scheduledTaskIntervalUnitLabel")}
              value={draft.intervalUnit}
              onChange={(event) => onChange({ intervalUnit: event.currentTarget.value as ScheduledTaskIntervalUnit })}
            >
              {INTERVAL_UNITS.map((unit) => (
                <option key={unit} value={unit}>
                  {unitLabel(unit, t)}
                </option>
              ))}
            </select>
          </div>
        ) : (
          <MomentControl
            mode={momentMode}
            onceAt={draft.onceAt}
            clock={draft.time}
            timeZone={draft.timeZone !== Intl.DateTimeFormat().resolvedOptions().timeZone ? draft.timeZone : undefined}
            month={draft.month}
            day={draft.day}
            invalid={Boolean(errors.onceAt || errors.time)}
            t={t}
            onChange={onChange}
          />
        )}
        {showEffectiveRange ? (
          <EffectiveRangeControl
            start={draft.startsDate}
            end={draft.expiresDate}
            invalid={Boolean(errors.effectiveRange)}
            t={t}
            onChange={onChange}
          />
        ) : null}
      </div>
      {error ? (
        <p className={styles.scheduleError} role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

function unitLabel(unit: ScheduledTaskIntervalUnit, t: TranslateFn): string {
  switch (unit) {
    case "minute":
      return t("scheduledTaskIntervalMinute");
    case "day":
      return t("scheduledTaskIntervalDay");
    default:
      return t("scheduledTaskIntervalHour");
  }
}

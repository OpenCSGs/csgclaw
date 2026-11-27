import { Check, ChevronDown, ChevronRight } from "lucide-react";
import { useRef, useState } from "react";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import {
  SCHEDULED_TASK_WEEKDAY_ORDER,
  scheduledTaskRecurrenceLabel,
  type ScheduledTaskRecurrence,
  weekdayLabel,
} from "@/models/scheduledTasks";
import { classNames } from "@/shared/lib/classNames";
import { handoffWheelToDialog, useWheelScroll } from "./useWheelScroll";
import styles from "./ScheduleControls.module.css";

type FlyoutKind = "weekdays" | "monthdays";

const RECURRENCE_ORDER: ScheduledTaskRecurrence[] = [
  "interval",
  "once",
  "daily",
  "weekly",
  "biweekly",
  "monthly",
  "yearly",
];

const FLYOUT_BY_RECURRENCE: Partial<Record<ScheduledTaskRecurrence, FlyoutKind>> = {
  weekly: "weekdays",
  biweekly: "weekdays",
  monthly: "monthdays",
};

type RecurrencePickerProps = {
  value: ScheduledTaskRecurrence;

  summary: string;
  weekdays: number[];
  monthDays: number[];
  invalid?: boolean;
  t: TranslateFn;
  onChangeType: (value: ScheduledTaskRecurrence) => void;
  onToggleWeekday: (weekday: number) => void;
  onToggleMonthDay: (day: number) => void;
};

export function RecurrencePicker({
  value,
  summary,
  weekdays,
  monthDays,
  invalid,
  t,
  onChangeType,
  onToggleWeekday,
  onToggleMonthDay,
}: RecurrencePickerProps) {
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const [open, setOpen] = useState(false);
  const [flyout, setFlyout] = useState<FlyoutKind | null>(() => FLYOUT_BY_RECURRENCE[value] ?? null);

  const flyoutListRef = useRef<HTMLDivElement | null>(null);
  useWheelScroll(flyoutListRef, flyout);

  function chooseType(next: ScheduledTaskRecurrence) {
    onChangeType(next);
    const nextFlyout = FLYOUT_BY_RECURRENCE[next] ?? null;
    setFlyout(nextFlyout);

    if (!nextFlyout) {
      setOpen(false);
    }
  }

  return (
    <PopoverRoot
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (nextOpen) {
          setFlyout(FLYOUT_BY_RECURRENCE[value] ?? null);
        }
      }}
    >
      <PopoverTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          className={classNames(styles.trigger, invalid && styles.triggerInvalid)}
          aria-label={t("scheduledTaskRecurrenceLabel")}
          aria-haspopup="dialog"
          aria-expanded={open}
          data-state={open ? "open" : "closed"}
          title={`${t("scheduledTaskRecurrenceLabel")}：${summary}`}
        >
          <span className={styles.triggerLabel}>{t("scheduledTaskRecurrenceLabel")}</span>
          <span className={styles.triggerValue}>{summary}</span>
          <ChevronDown size={15} aria-hidden="true" className={styles.triggerChevron} />
        </button>
      </PopoverTrigger>
      <PopoverContent
        onWheel={(event) => handoffWheelToDialog(event, triggerRef.current)}
        align="start"
        className={styles.pickerPopover}
        sideOffset={6}
        onFocusOutside={(event) => event.preventDefault()}
      >
        <div className={styles.picker}>
          <div className={styles.pickerList} role="listbox" aria-label={t("scheduledTaskRecurrenceLabel")}>
            {RECURRENCE_ORDER.map((item) => {
              const itemFlyout = FLYOUT_BY_RECURRENCE[item];
              const active = item === value;
              return (
                <div
                  key={item}
                  className={classNames(styles.pickerRow, active && styles.pickerRowActive)}
                  data-active={active ? "true" : undefined}
                  data-flyout={itemFlyout === flyout && itemFlyout ? "true" : undefined}
                >
                  <button
                    type="button"
                    role="option"
                    aria-selected={active}
                    className={styles.pickerRowMain}
                    onClick={() => chooseType(item)}
                  >
                    {scheduledTaskRecurrenceLabel(item, t)}
                  </button>
                  {itemFlyout ? (
                    <button
                      type="button"
                      className={styles.pickerRowCaret}
                      aria-label={scheduledTaskRecurrenceLabel(item, t)}
                      aria-expanded={itemFlyout === flyout}
                      onClick={() => {
                        onChangeType(item);
                        setFlyout((current) => (current === itemFlyout ? null : itemFlyout));
                      }}
                    >
                      <ChevronRight size={14} aria-hidden="true" />
                    </button>
                  ) : null}
                </div>
              );
            })}
          </div>
          {flyout ? (
            <div className={styles.pickerFlyout}>
              <p className={styles.flyoutHint}>
                {flyout === "weekdays" ? t("scheduledTaskWeekdayPickHint") : t("scheduledTaskMonthDayPickHint")}
              </p>
              <div className={styles.flyoutList} role="listbox" aria-multiselectable="true" ref={flyoutListRef}>
                {flyout === "weekdays"
                  ? SCHEDULED_TASK_WEEKDAY_ORDER.map((weekday) => (
                      <button
                        key={weekday}
                        type="button"
                        role="option"
                        aria-selected={weekdays.includes(weekday)}
                        className={styles.flyoutOption}
                        data-selected={weekdays.includes(weekday) ? "true" : undefined}
                        onClick={() => onToggleWeekday(weekday)}
                      >
                        <span>{weekdayLabel(weekday, t)}</span>
                        {weekdays.includes(weekday) ? <Check size={14} aria-hidden="true" /> : null}
                      </button>
                    ))
                  : Array.from({ length: 31 }, (_, index) => index + 1).map((day) => (
                      <button
                        key={day}
                        type="button"
                        role="option"
                        aria-selected={monthDays.includes(day)}
                        className={styles.flyoutOption}
                        data-selected={monthDays.includes(day) ? "true" : undefined}
                        onClick={() => onToggleMonthDay(day)}
                      >
                        <span>{t("scheduledTaskMonthDay", { day })}</span>
                        {monthDays.includes(day) ? <Check size={14} aria-hidden="true" /> : null}
                      </button>
                    ))}
              </div>
            </div>
          ) : null}
        </div>
      </PopoverContent>
    </PopoverRoot>
  );
}

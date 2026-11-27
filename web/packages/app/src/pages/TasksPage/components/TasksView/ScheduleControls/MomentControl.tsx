import { CalendarDays, ChevronDown, Clock } from "lucide-react";
import { useRef, useState } from "react";
import { Button, PopoverContent, PopoverRoot, PopoverTrigger } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { classNames } from "@/shared/lib/classNames";
import { CalendarMonth } from "./CalendarMonth";
import { displayDate, displayDateTime, localDateISO, pad2, parseClock, parseISODate } from "./calendarUtils";
import { TimeColumns } from "./TimeColumns";
import { handoffWheelToDialog } from "./useWheelScroll";
import styles from "./ScheduleControls.module.css";

export type MomentMode = "time" | "datetime" | "yearly";

type MomentControlProps = {
  mode: MomentMode;

  onceAt: string;

  clock: string;
  timeZone?: string;
  month: number;
  day: number;
  invalid?: boolean;
  t: TranslateFn;

  onChange: (patch: { onceAt?: string; time?: string; month?: number; day?: number }) => void;
};

export function MomentControl({ mode, onceAt, clock, timeZone, month, day, invalid, t, onChange }: MomentControlProps) {
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const [open, setOpen] = useState(false);
  const parsedClock = parseClock(mode === "datetime" ? onceAt.slice(11) : clock) ?? { hour: 9, minute: 0 };
  const [draftHour, setDraftHour] = useState(parsedClock.hour);
  const [draftMinute, setDraftMinute] = useState(parsedClock.minute);
  const [draftDate, setDraftDate] = useState(() => onceAt.slice(0, 10) || localDateISO(new Date()));
  const [draftMonth, setDraftMonth] = useState(month || new Date().getMonth() + 1);
  const [draftDay, setDraftDay] = useState(day || new Date().getDate());

  function resetDraft() {
    const current = parseClock(mode === "datetime" ? onceAt.slice(11) : clock) ?? { hour: 9, minute: 0 };
    setDraftHour(current.hour);
    setDraftMinute(current.minute);
    setDraftDate(onceAt.slice(0, 10) || localDateISO(new Date()));
    const today = new Date();
    setDraftMonth(month || today.getMonth() + 1);
    setDraftDay(day || today.getDate());
  }

  function confirm() {
    const nextClock = `${pad2(draftHour)}:${pad2(draftMinute)}`;
    if (mode === "datetime") {
      onChange({ onceAt: `${draftDate}T${nextClock}`, time: nextClock });
    } else if (mode === "yearly") {
      onChange({ month: draftMonth, day: draftDay, time: nextClock });
    } else {
      onChange({ time: nextClock });
    }
    setOpen(false);
  }

  const triggerValue =
    mode === "datetime"
      ? displayDateTime(onceAt)
      : mode === "yearly"
        ? `${t("scheduledTaskYearDay", { month, day })} ${clock}`.trim()
        : clock;

  const displayedValue =
    triggerValue && timeZone && mode !== "datetime" ? `${triggerValue} (${timeZone})` : triggerValue;
  const triggerLabel = mode === "time" ? t("scheduledTaskTimeLabel") : t("scheduledTaskDateLabel");

  const calendarValue =
    mode === "datetime" ? draftDate : mode === "yearly" ? `2000-${pad2(draftMonth)}-${pad2(draftDay)}` : "";

  return (
    <PopoverRoot
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (nextOpen) {
          resetDraft();
        }
      }}
    >
      <PopoverTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          className={classNames(styles.trigger, invalid && styles.triggerInvalid)}
          aria-label={triggerLabel}
          aria-haspopup="dialog"
          aria-expanded={open}
          data-state={open ? "open" : "closed"}
          title={`${triggerLabel}：${displayedValue}`}
        >
          <span className={styles.triggerLabel}>{triggerLabel}</span>
          <span className={styles.triggerValue} data-empty={triggerValue ? undefined : "true"}>
            {displayedValue || displayDate("") || "—"}
          </span>
          {mode === "time" ? (
            <ChevronDown size={15} aria-hidden="true" className={styles.triggerChevron} />
          ) : (
            <CalendarDays size={14} aria-hidden="true" className={styles.triggerChevron} />
          )}
        </button>
      </PopoverTrigger>
      <PopoverContent
        onWheel={(event) => handoffWheelToDialog(event, triggerRef.current)}
        aria-label={triggerLabel}
        align="start"
        className={styles.momentPopover}
        sideOffset={6}
        onFocusOutside={(event) => event.preventDefault()}
      >
        <div className={styles.momentBody}>
          {mode === "time" ? null : (
            <CalendarMonth
              value={calendarValue}
              t={t}
              onSelect={(iso) => {
                if (mode === "datetime") {
                  setDraftDate(iso);
                  return;
                }
                const parts = parseISODate(iso);
                if (parts) {
                  setDraftMonth(parts.month);
                  setDraftDay(parts.day);
                }
              }}
            />
          )}
          <div className={styles.momentTime}>
            <p className={styles.momentTimeHint}>
              <Clock size={13} aria-hidden="true" />
              {t("scheduledTaskTimeLabel")}
              <strong>{`${pad2(draftHour)}:${pad2(draftMinute)}`}</strong>
            </p>
            <TimeColumns
              hour={draftHour}
              hourLabel={t("scheduledTaskHourColumn")}
              minute={draftMinute}
              minuteLabel={t("scheduledTaskMinuteColumn")}
              onSelect={(hour, minute) => {
                setDraftHour(hour);
                setDraftMinute(minute);
              }}
            />
          </div>
        </div>
        <div className={styles.momentFooter}>
          <span className={styles.momentSummary}>
            {mode === "datetime"
              ? `${displayDate(draftDate)} ${pad2(draftHour)}:${pad2(draftMinute)}`
              : mode === "yearly"
                ? `${t("scheduledTaskYearDay", { month: draftMonth, day: draftDay })} ${pad2(draftHour)}:${pad2(draftMinute)}`
                : `${pad2(draftHour)}:${pad2(draftMinute)}`}
          </span>
          <Button variant="primary" size="sm" onClick={confirm}>
            {t("confirm")}
          </Button>
        </div>
      </PopoverContent>
    </PopoverRoot>
  );
}

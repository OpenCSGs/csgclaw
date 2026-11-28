import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from "lucide-react";
import { useEffect, useState } from "react";
import type { TranslateFn } from "@/models/conversations";
import { SCHEDULED_TASK_WEEKDAY_ORDER } from "@/models/scheduledTasks";
import { monthCells, monthLabel, parseISODate, shiftMonthView, weekdayShort } from "./calendarUtils";
import type { MonthView } from "./calendarUtils";
import styles from "./ScheduleControls.module.css";

type CalendarMonthProps = {
  value: string;

  rangeEnd?: string;
  onSelect: (iso: string) => void;
  t: TranslateFn;
};

export function CalendarMonth({ value, rangeEnd, onSelect, t }: CalendarMonthProps) {
  const selected = parseISODate(value);
  const [view, setView] = useState<MonthView>(() =>
    selected
      ? { year: selected.year, month: selected.month - 1 }
      : { year: new Date().getFullYear(), month: new Date().getMonth() },
  );

  useEffect(() => {
    const next = parseISODate(value);
    if (next) {
      setView((current) =>
        current.year === next.year && current.month === next.month - 1
          ? current
          : { year: next.year, month: next.month - 1 },
      );
    }
  }, [value]);

  const cells = monthCells(view);

  return (
    <div className={styles.calendar}>
      <div className={styles.calendarHeader}>
        <button
          type="button"
          className={styles.calendarNav}
          aria-label={t("scheduledTaskPreviousYear")}
          onClick={() => setView((current) => shiftMonthView(current, -12))}
        >
          <ChevronsLeft size={15} aria-hidden="true" />
        </button>
        <button
          type="button"
          className={styles.calendarNav}
          aria-label={t("scheduledTaskPreviousMonth")}
          onClick={() => setView((current) => shiftMonthView(current, -1))}
        >
          <ChevronLeft size={15} aria-hidden="true" />
        </button>
        <strong aria-live="polite">{monthLabel(view)}</strong>
        <button
          type="button"
          className={styles.calendarNav}
          aria-label={t("scheduledTaskNextMonth")}
          onClick={() => setView((current) => shiftMonthView(current, 1))}
        >
          <ChevronRight size={15} aria-hidden="true" />
        </button>
        <button
          type="button"
          className={styles.calendarNav}
          aria-label={t("scheduledTaskNextYear")}
          onClick={() => setView((current) => shiftMonthView(current, 12))}
        >
          <ChevronsRight size={15} aria-hidden="true" />
        </button>
      </div>
      <div className={styles.calendarWeekdays} aria-hidden="true">
        {SCHEDULED_TASK_WEEKDAY_ORDER.map((weekday) => (
          <span key={weekday}>{weekdayShort(weekday, t)}</span>
        ))}
      </div>
      <div className={styles.calendarGrid} role="grid">
        {cells.map((cell) => {
          const isStart = cell.iso === value;
          const isEnd = Boolean(rangeEnd) && cell.iso === rangeEnd;
          const inRange = Boolean(rangeEnd) && cell.iso > value && cell.iso < rangeEnd!;
          return (
            <button
              key={cell.iso}
              type="button"
              role="gridcell"
              className={styles.calendarDay}
              data-outside={cell.inMonth ? undefined : "true"}
              data-weekend={cell.weekday === 0 || cell.weekday === 6 ? "true" : undefined}
              data-selected={isStart || isEnd ? "true" : undefined}
              data-in-range={inRange ? "true" : undefined}
              aria-pressed={isStart || isEnd}
              aria-label={cell.iso}
              onClick={() => onSelect(cell.iso)}
            >
              {cell.day}
            </button>
          );
        })}
      </div>
    </div>
  );
}

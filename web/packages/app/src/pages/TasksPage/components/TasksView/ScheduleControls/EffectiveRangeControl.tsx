import { CalendarRange, Check, ChevronDown } from "lucide-react";
import { useState } from "react";
import { PopoverContent, PopoverRoot, PopoverTrigger } from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { classNames } from "@/shared/lib/classNames";
import { CalendarMonth } from "./CalendarMonth";
import { compactRange, displayDate } from "./calendarUtils";
import styles from "./ScheduleControls.module.css";

type EffectiveRangeControlProps = {
  start: string;

  end: string;
  invalid?: boolean;
  t: TranslateFn;
  onChange: (patch: { startsDate?: string; expiresDate?: string }) => void;
};

export function EffectiveRangeControl({ start, end, invalid, t, onChange }: EffectiveRangeControlProps) {
  const rangeComplete = Boolean(end);
  const [open, setOpen] = useState(false);
  const [rangeMode, setRangeMode] = useState(rangeComplete);

  const [pendingStart, setPendingStart] = useState(start);
  const [pendingEnd, setPendingEnd] = useState(end);

  function summary(): string {
    return compactRange(start, end, t("scheduledTaskEffectiveForever"), t("scheduledTaskEffectiveFrom"));
  }

  function fullSummary(): string {
    if (!rangeComplete) {
      return t("scheduledTaskEffectiveForever");
    }
    return `${start ? displayDate(start) : t("scheduledTaskEffectiveASAP")} ~ ${displayDate(end)}`;
  }

  function pick(iso: string) {
    if (!pendingStart || pendingEnd) {
      setPendingStart(iso);
      setPendingEnd("");
      return;
    }
    const [from, to] = iso < pendingStart ? [iso, pendingStart] : [pendingStart, iso];
    setPendingStart(from);
    setPendingEnd(to);
    onChange({ startsDate: from, expiresDate: to });

    setOpen(false);
  }

  return (
    <PopoverRoot
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        setRangeMode(rangeComplete);
        setPendingStart(start);
        setPendingEnd(end);
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          className={classNames(styles.trigger, invalid && styles.triggerInvalid)}
          aria-label={t("scheduledTaskEffectiveLabel")}
          aria-haspopup="dialog"
          aria-expanded={open}
          data-state={open ? "open" : "closed"}
          title={`${t("scheduledTaskEffectiveLabel")}：${fullSummary()}`}
        >
          <span className={styles.triggerLabel}>{t("scheduledTaskEffectiveLabel")}</span>
          <span className={styles.triggerValue}>{summary()}</span>
          <CalendarRange size={14} aria-hidden="true" className={styles.triggerChevron} />
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className={styles.expiryPopover}
        sideOffset={6}
        onFocusOutside={(event) => event.preventDefault()}
      >
        <div className={styles.pickerList}>
          <button
            type="button"
            role="option"
            aria-selected={!rangeComplete}
            className={styles.flyoutOption}
            data-selected={!rangeComplete ? "true" : undefined}
            onClick={() => {
              setRangeMode(false);
              setPendingStart("");
              setPendingEnd("");
              onChange({ startsDate: "", expiresDate: "" });
              setOpen(false);
            }}
          >
            <span>{t("scheduledTaskEffectiveForever")}</span>
            {!rangeComplete ? <Check size={14} aria-hidden="true" /> : null}
          </button>
          <button
            type="button"
            role="option"
            aria-selected={rangeComplete}
            className={styles.flyoutOption}
            data-selected={rangeComplete ? "true" : undefined}
            onClick={() => {
              setRangeMode(true);
              setPendingStart("");
              setPendingEnd("");
            }}
          >
            <span>{t("scheduledTaskEffectiveRange")}</span>
            {rangeComplete ? <Check size={14} aria-hidden="true" /> : null}
          </button>
        </div>
        {rangeMode ? (
          <div className={styles.expiryCalendar}>
            <p className={styles.flyoutHint}>{t("scheduledTaskEffectiveRangeHint")}</p>
            <CalendarMonth value={pendingStart} rangeEnd={pendingEnd} t={t} onSelect={pick} />
          </div>
        ) : (
          <p className={styles.expiryHint}>
            <CalendarRange size={13} aria-hidden="true" />
            {t("scheduledTaskEffectiveForeverHint")}
          </p>
        )}
        {rangeMode && pendingStart && !pendingEnd ? (
          <p className={styles.expiryPending}>
            <ChevronDown size={12} aria-hidden="true" />
            {t("scheduledTaskEffectivePickEnd", { date: displayDate(pendingStart) })}
          </p>
        ) : null}
      </PopoverContent>
    </PopoverRoot>
  );
}

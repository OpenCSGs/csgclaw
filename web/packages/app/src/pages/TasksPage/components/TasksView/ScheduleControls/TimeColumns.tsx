import { useEffect, useRef, type KeyboardEvent } from "react";
import { pad2 } from "./calendarUtils";
import { useWheelScroll } from "./useWheelScroll";
import styles from "./ScheduleControls.module.css";

type TimeColumnsProps = {
  hour: number;
  hourLabel: string;
  minute: number;
  minuteLabel: string;
  onSelect: (hour: number, minute: number) => void;
};

const HOURS = Array.from({ length: 24 }, (_, index) => index);
const MINUTES = Array.from({ length: 60 }, (_, index) => index);

export function TimeColumns({ hour, hourLabel, minute, minuteLabel, onSelect }: TimeColumnsProps) {
  const hourListRef = useRef<HTMLDivElement | null>(null);
  const minuteListRef = useRef<HTMLDivElement | null>(null);

  useWheelScroll(hourListRef);
  useWheelScroll(minuteListRef);

  useEffect(() => {
    hourListRef.current?.querySelector<HTMLElement>('[data-selected="true"]')?.scrollIntoView({ block: "center" });
    minuteListRef.current?.querySelector<HTMLElement>('[data-selected="true"]')?.scrollIntoView({ block: "center" });
  }, []);

  function navigate(
    event: KeyboardEvent<HTMLDivElement>,
    current: number,
    max: number,
    select: (value: number) => void,
  ) {
    const target = { ArrowUp: current - 1, ArrowDown: current + 1, Home: 0, End: max }[event.key];
    if (target === undefined) return;
    event.preventDefault();
    const next = Math.max(0, Math.min(max, target));
    select(next);
    event.currentTarget.querySelectorAll("button")[next]?.focus();
  }

  return (
    <div className={styles.timeColumns}>
      <div
        className={styles.timeColumn}
        ref={hourListRef}
        role="listbox"
        aria-label={hourLabel}
        onKeyDown={(event) => navigate(event, hour, 23, (value) => onSelect(value, minute))}
      >
        {HOURS.map((value) => (
          <button
            key={value}
            type="button"
            role="option"
            aria-selected={value === hour}
            tabIndex={value === hour ? 0 : -1}
            className={styles.timeCell}
            data-selected={value === hour ? "true" : undefined}
            onClick={() => onSelect(value, minute)}
          >
            {pad2(value)}
          </button>
        ))}
      </div>
      <div
        className={styles.timeColumn}
        ref={minuteListRef}
        role="listbox"
        aria-label={minuteLabel}
        onKeyDown={(event) => navigate(event, minute, 59, (value) => onSelect(hour, value))}
      >
        {MINUTES.map((value) => (
          <button
            key={value}
            type="button"
            role="option"
            aria-selected={value === minute}
            tabIndex={value === minute ? 0 : -1}
            className={styles.timeCell}
            data-selected={value === minute ? "true" : undefined}
            onClick={() => onSelect(hour, value)}
          >
            {pad2(value)}
          </button>
        ))}
      </div>
    </div>
  );
}

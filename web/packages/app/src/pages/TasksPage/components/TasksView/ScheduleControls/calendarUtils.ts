import type { TranslateFn } from "@/models/conversations";
import { weekdayLabel } from "@/models/scheduledTasks";

export function pad2(value: number): string {
  return String(value).padStart(2, "0");
}

export function localDateISO(date: Date): string {
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`;
}

export function localDateTimeInput(date: Date): string {
  return `${localDateISO(date)}T${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
}

export function clockText(hour: number, minute: number): string {
  return `${pad2(hour)}:${pad2(minute)}`;
}

export function displayDateTime(iso: string): string {
  const date = parseLocalDateTime(iso);
  if (!date) {
    return "";
  }
  return `${date.getFullYear()}/${pad2(date.getMonth() + 1)}/${pad2(date.getDate())} ${clockText(
    date.getHours(),
    date.getMinutes(),
  )}`;
}

export function displayDate(iso: string): string {
  const parts = parseISODate(iso);
  return parts ? `${parts.year}/${pad2(parts.month)}/${pad2(parts.day)}` : "";
}

export function compactRange(startISO: string, endISO: string, foreverLabel: string, fromLabel: string): string {
  const start = parseISODate(startISO);
  const end = parseISODate(endISO);
  if (!start && !end) {
    return foreverLabel;
  }
  if (!end && start) {
    return `${displayDate(startISO)} ${fromLabel}`;
  }
  if (!start && end) {
    return `~ ${displayDate(endISO)}`;
  }
  const sameYear = start!.year === end!.year;
  const left = sameYear ? `${start!.year}/${pad2(start!.month)}/${pad2(start!.day)}` : displayDate(startISO);
  const right = sameYear ? `${pad2(end!.month)}/${pad2(end!.day)}` : displayDate(endISO);
  return `${left} ~ ${right}`;
}

export function parseISODate(value: string): { year: number; month: number; day: number } | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value || "");
  if (!match) {
    return null;
  }
  return { year: Number(match[1]), month: Number(match[2]), day: Number(match[3]) };
}

export function parseLocalDateTime(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(value || "");
  if (!match) {
    return null;
  }
  const parsed = new Date(
    Number(match[1]),
    Number(match[2]) - 1,
    Number(match[3]),
    Number(match[4]),
    Number(match[5]),
    0,
    0,
  );
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

export function parseClock(value: string): { hour: number; minute: number } | null {
  const match = /^(\d{1,2}):(\d{2})$/.exec(value || "");
  if (!match) {
    return null;
  }
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (hour > 23 || minute > 59) {
    return null;
  }
  return { hour, minute };
}

export type MonthView = { year: number; month: number };

export function monthCells(view: MonthView): Array<{ iso: string; day: number; inMonth: boolean; weekday: number }> {
  const first = new Date(view.year, view.month, 1);
  const offset = (first.getDay() + 6) % 7;
  return Array.from({ length: 42 }, (_, index) => {
    const date = new Date(view.year, view.month, index - offset + 1);
    return {
      iso: localDateISO(date),
      day: date.getDate(),
      inMonth: date.getMonth() === view.month,
      weekday: date.getDay(),
    };
  });
}

export function shiftMonthView(view: MonthView, delta: number): MonthView {
  const date = new Date(view.year, view.month + delta, 1);
  return { year: date.getFullYear(), month: date.getMonth() };
}

export function weekdayShort(weekday: number, t: TranslateFn): string {
  const full = weekdayLabel(weekday, t);
  return full.startsWith("周") ? full.slice(1) : full.slice(0, 2);
}

export function monthLabel(view: MonthView): string {
  const locale = typeof document !== "undefined" && document.documentElement.lang === "en" ? "en-US" : "zh-CN";
  return new Intl.DateTimeFormat(locale, { year: "numeric", month: "long" }).format(new Date(view.year, view.month, 1));
}

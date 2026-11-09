import { useId, useRef, useState } from "react";
import { Button, Field, PopoverRoot, PopoverTrigger, PopoverContent, TextInput } from "@/components/ui";
import { useLabTranslation } from "../LabLocale";
import { monthDays, shiftMonth } from "./calendarModel";
import styles from "./ApplicationFoundations.module.css";
function CalendarGrid({ selected, onSelect }: { selected: string; onSelect: (date: string) => void }) {
  const { text: t, locale } = useLabTranslation();
  const [view, setView] = useState(() => ({
    year: Number(selected.slice(0, 4)),
    month: Number(selected.slice(5, 7)) - 1,
  }));
  const buttons = useRef<(HTMLButtonElement | null)[]>([]);
  const days = monthDays(view.year, view.month);
  const language = locale === "zh" ? "zh-CN" : "en-US";
  const month = new Intl.DateTimeFormat(language, { year: "numeric", month: "long", timeZone: "UTC" }).format(
    new Date(Date.UTC(view.year, view.month, 1)),
  );
  return (
    <div className={styles.calendar}>
      <header className={styles.calendarHeader}>
        <Button aria-label={t("上个月")} onClick={() => setView(shiftMonth(view.year, view.month, -1))}>
          {t("上个月")}
        </Button>
        <strong aria-live="polite">{month}</strong>
        <Button aria-label={t("下个月")} onClick={() => setView(shiftMonth(view.year, view.month, 1))}>
          {t("下个月")}
        </Button>
      </header>
      <table className={styles.calendarTable}>
        <caption className={styles.srOnly}>{t("选择日期")}</caption>
        <thead>
          <tr>
            {Array.from({ length: 7 }, (_, i) => (
              <th key={i} scope="col">
                {new Intl.DateTimeFormat(language, { weekday: "short", timeZone: "UTC" }).format(
                  new Date(Date.UTC(2025, 0, 6 + i)),
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: 6 }, (_, row) => (
            <tr key={row}>
              {days.slice(row * 7, row * 7 + 7).map((date, col) => {
                const index = row * 7 + col;
                return (
                  <td key={date.iso}>
                    <button
                      type="button"
                      ref={(el) => {
                        buttons.current[index] = el;
                      }}
                      data-outside={!date.inMonth}
                      aria-label={date.iso}
                      aria-pressed={selected === date.iso}
                      tabIndex={
                        selected === date.iso || (!days.some((d) => d.iso === selected) && index === 7) ? 0 : -1
                      }
                      onKeyDown={(event) => {
                        const delta: Record<string, number> = {
                          ArrowLeft: -1,
                          ArrowRight: 1,
                          ArrowUp: -7,
                          ArrowDown: 7,
                          Home: -col,
                          End: 6 - col,
                        };
                        if (event.key in delta) {
                          event.preventDefault();
                          buttons.current[Math.max(0, Math.min(41, index + delta[event.key]))]?.focus();
                        }
                      }}
                      onClick={() => onSelect(date.iso)}
                    >
                      {date.day}
                    </button>
                  </td>
                );
              })}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
export function DatePickerDemo() {
  const { text: t } = useLabTranslation();
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("2025-01-10");
  const [draft, setDraft] = useState(value);
  return (
    <>
      <PopoverRoot
        open={open}
        onOpenChange={(next) => {
          if (next) setDraft(value);
          setOpen(next);
        }}
      >
        <PopoverTrigger asChild>
          <Button>
            {t("选择日期")}：{value}
          </Button>
        </PopoverTrigger>
        <PopoverContent className={`${styles.demoSurface} ${styles.datePopover}`}>
          <CalendarGrid selected={draft} onSelect={setDraft} />
          <p>
            {t("待确认日期")}：{draft}
          </p>
          <div className={styles.actions}>
            <Button onClick={() => setOpen(false)}>{t("取消")}</Button>
            <Button
              variant="primary"
              onClick={() => {
                setValue(draft);
                setOpen(false);
              }}
            >
              {t("应用日期")}
            </Button>
          </div>
        </PopoverContent>
      </PopoverRoot>
      <p role="status">
        {t("已确认日期")}：{value}
      </p>
    </>
  );
}
export function CalendarDemo() {
  const { text: t } = useLabTranslation();
  const [selected, setSelected] = useState("2025-01-10");
  const [title, setTitle] = useState("");
  const [events, setEvents] = useState([{ date: "2025-01-10", title: "设计评审" }]);
  const [error, setError] = useState(false);
  const id = useId();
  return (
    <>
      <CalendarGrid selected={selected} onSelect={setSelected} />
      <h4>{selected}</h4>
      <ul>
        {events
          .filter((event) => event.date === selected)
          .map((event, i) => (
            <li key={i}>{event.title === "设计评审" ? t(event.title) : event.title}</li>
          ))}
      </ul>
      {!events.some((event) => event.date === selected) && <p>{t("当天没有日程。")}</p>}
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (!title.trim()) {
            setError(true);
            return;
          }
          setEvents([...events, { date: selected, title: title.trim() }]);
          setTitle("");
          setError(false);
        }}
      >
        <Field label={t("日程名称")} error={error ? t("请输入日程名称。") : undefined}>
          <TextInput
            id={id}
            value={title}
            onChange={(event) => {
              setTitle(event.target.value);
              setError(false);
            }}
          />
        </Field>
        <Button type="submit" variant="primary">
          {t("添加日程")}
        </Button>
      </form>
      <p role="status">
        {t("日程总数")}：{events.length}
      </p>
    </>
  );
}

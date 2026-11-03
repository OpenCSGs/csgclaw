import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { CalendarClock } from "lucide-react";
import { Button, TextArea } from "@/components/ui";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import type { AgentLike } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { executorName, scheduledTaskExecutors, scheduledTaskMention } from "@/models/scheduledTasks";
import styles from "./ScheduledTaskStart.module.css";

type Props = {
  agents: AgentLike[];
  t: TranslateFn;
  busy?: boolean;
  error?: string;
  focusToken: number;
  onSend?: (text: string) => Promise<boolean>;
};
export function ScheduledTaskStart({ agents, t, busy = false, error = "", focusToken, onSend }: Props) {
  const [text, setText] = useState("");
  const [caret, setCaret] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const [active, setActive] = useState(0);
  const editor = useRef<HTMLTextAreaElement>(null);
  const pendingSelection = useRef<number | null>(null);
  useLayoutEffect(() => {
    if (pendingSelection.current === null) return;
    editor.current?.focus();
    editor.current?.setSelectionRange(pendingSelection.current, pendingSelection.current);
    pendingSelection.current = null;
  }, [text]);
  const mention = dismissed ? null : scheduledTaskMention(text, caret);
  const query = mention?.query.toLocaleLowerCase();
  const candidates = useMemo(
    () =>
      query === undefined
        ? []
        : scheduledTaskExecutors(agents)
            .filter(
              (agent) =>
                executorName(agent).toLocaleLowerCase().includes(query) ||
                String(agent.id).toLocaleLowerCase().includes(query),
            )
            .slice(0, 8),
    [agents, query],
  );
  useEffect(() => {
    if (focusToken) editor.current?.focus();
  }, [focusToken]);
  function choose(agent: AgentLike) {
    if (!mention) return;
    const inserted = `@${executorName(agent)} `;
    const position = mention.start + inserted.length;
    pendingSelection.current = position;
    setText(text.slice(0, mention.start) + inserted + text.slice(mention.end));
    setCaret(position);
    setDismissed(true);
  }
  async function submit() {
    if (!text.trim() || busy || !onSend) return;
    if (await onSend(text.trim())) {
      setText("");
      setCaret(0);
    }
  }
  return (
    <div className={styles.start}>
      <div className={styles.inner}>
        <div className={styles.intro}>
          <span className={styles.icon}>
            <CalendarClock size={26} aria-hidden="true" />
          </span>
          <h2>{t("scheduledTaskTextTitle")}</h2>
          <p>{t("scheduledTaskTextBody")}</p>
        </div>
        <div key={focusToken} className={styles.composer} data-focus={focusToken > 0 || undefined}>
          <TextArea
            ref={editor}
            rows={4}
            value={text}
            disabled={busy}
            aria-label={t("scheduledTaskTextInput")}
            placeholder={t("scheduledTaskTextPlaceholder")}
            aria-controls={candidates.length ? "scheduled-executor-options" : undefined}
            aria-expanded={candidates.length > 0}
            aria-activedescendant={
              candidates.length ? `scheduled-executor-${Math.min(active, candidates.length - 1)}` : undefined
            }
            onChange={(event) => {
              setText(event.currentTarget.value);
              setCaret(event.currentTarget.selectionStart);
              setDismissed(false);
              setActive(0);
            }}
            onSelect={(event) => setCaret(event.currentTarget.selectionStart)}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing || !candidates.length) return;
              if (event.key === "Escape") {
                event.preventDefault();
                setDismissed(true);
              }
              if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                event.preventDefault();
                setActive(
                  (current) => (current + (event.key === "ArrowDown" ? 1 : -1) + candidates.length) % candidates.length,
                );
              }
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                choose(candidates[Math.min(active, candidates.length - 1)]);
              }
            }}
          />
          {candidates.length ? (
            <div
              id="scheduled-executor-options"
              role="listbox"
              aria-label={t("scheduledTaskAgentLabel")}
              className={styles.mentions}
            >
              {candidates.map((agent, index) => (
                <button
                  key={agent.id}
                  id={`scheduled-executor-${index}`}
                  type="button"
                  role="option"
                  aria-selected={active === index}
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => choose(agent)}
                >
                  <span className={styles.avatar} aria-hidden="true">
                    <AgentAvatarContent avatar={agent.avatar} fallback={executorName(agent).slice(0, 1)} />
                  </span>
                  <span>{executorName(agent)}</span>
                </button>
              ))}
            </div>
          ) : null}
          <div className={styles.actions}>
            <span>{t("scheduledTaskTextEnterHint")}</span>
            <Button
              variant="primary"
              size="sm"
              loading={busy}
              disabled={busy || !text.trim() || !onSend}
              onClick={() => void submit()}
            >
              {t("scheduledTaskTextSend")}
            </Button>
          </div>
        </div>
        {error ? (
          <p role="alert" className="form-error">
            {error}
          </p>
        ) : null}
        <ul className={styles.tips}>
          <li>{t("scheduledTaskTextTipExecutor")}</li>
          <li>{t("scheduledTaskTextTipManual")}</li>
          <li>{t("scheduledTaskTextTipDetails")}</li>
        </ul>
      </div>
    </div>
  );
}

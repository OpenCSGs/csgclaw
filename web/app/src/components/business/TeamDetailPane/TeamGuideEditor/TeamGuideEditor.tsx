import { useId, useLayoutEffect, useRef, useState } from "react";
import type { KeyboardEvent } from "react";
import { AtSign } from "lucide-react";
import { Button, TextArea } from "@/components/ui";
import { MentionPicker, type MentionPickerUser } from "@/components/business/ConversationPane";
import { getMentionCandidates } from "@/models/composer";
import type { TranslateFn } from "@/models/conversations";
import { guideMentionAt, insertGuideMention, type GuideMention } from "./guideMentions";
import styles from "./TeamGuideEditor.module.css";

export function TeamGuideEditor({
  value,
  savedValue,
  members,
  saving = false,
  onChange,
  onSave,
  t,
}: {
  value: string;
  savedValue: string;
  members: MentionPickerUser[];
  saving?: boolean;
  onChange: (value: string) => void;
  onSave?: (value: string) => Promise<boolean>;
  t: TranslateFn;
}) {
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const composing = useRef(false);
  const [mention, setMention] = useState<GuideMention | null>(null);
  const [activeIndex, setActiveIndex] = useState(0);
  const [pending, setPending] = useState(false);
  const [saveFailed, setSaveFailed] = useState(false);
  const hintID = useId();
  const busy = saving || pending;
  const disabled = busy || !onSave;
  const dirty = value !== savedValue;
  const candidates = mention
    ? (getMentionCandidates(members, mention.query, { limit: members.length }) as MentionPickerUser[])
    : [];

  useLayoutEffect(() => {
    const editor = editorRef.current;
    if (!editor) return;
    const fitContent = () => {
      const style = getComputedStyle(editor);
      const borders = (parseFloat(style.borderTopWidth) || 0) + (parseFloat(style.borderBottomWidth) || 0);
      editor.style.height = "0px";
      editor.style.height = `${editor.scrollHeight + borders}px`;
    };
    fitContent();
    let width = -1;
    const observer = new ResizeObserver(([entry]) => {
      if (entry && entry.contentRect.width !== width) {
        width = entry.contentRect.width;
        fitContent();
      }
    });
    observer.observe(editor);
    return () => observer.disconnect();
  }, [value]);

  function syncMention(editor: HTMLTextAreaElement) {
    setMention(composing.current ? null : guideMentionAt(editor.value, editor.selectionStart, editor.selectionEnd));
    setActiveIndex(0);
  }

  function applyMention(member: MentionPickerUser) {
    if (!mention || disabled) return;
    const next = insertGuideMention(value, mention, member.name || member.id);
    onChange(next.value);
    setMention(null);
    requestAnimationFrame(() => {
      editorRef.current?.focus();
      editorRef.current?.setSelectionRange(next.caret, next.caret);
    });
  }

  function handleKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (composing.current || event.nativeEvent.isComposing || event.keyCode === 229 || !mention) return;
    if (event.key === "Escape") {
      event.preventDefault();
      setMention(null);
    } else if (candidates.length && (event.key === "ArrowDown" || event.key === "ArrowUp")) {
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      setActiveIndex((current) => (current + step + candidates.length) % candidates.length);
    } else if (candidates.length && (event.key === "Enter" || event.key === "Tab")) {
      event.preventDefault();
      applyMention(candidates[activeIndex] || candidates[0]);
    }
  }

  async function save() {
    if (!onSave || busy || !dirty) return;
    setPending(true);
    setMention(null);
    setSaveFailed(false);
    try {
      setSaveFailed(!(await onSave(value)));
    } catch {
      setSaveFailed(true);
    } finally {
      setPending(false);
    }
  }

  return (
    <section
      className={styles.panel}
      role="tabpanel"
      id="team-detail-panel-guide"
      aria-labelledby="team-detail-tab-guide"
    >
      <div className={styles.header}>
        <div>
          <h2>{t("teamGuideTab")}</h2>
          <p>{t("teamGuideDescription")}</p>
        </div>
        <div className={styles.actions}>
          <span className={styles.state} role="status">
            {t(busy ? "teamSaving" : dirty ? "teamGuideDirty" : "teamGuideSaved")}
          </span>
          {onSave ? (
            <Button
              variant="primary"
              size="sm"
              disabled={!dirty || busy}
              loading={busy}
              loadingLabel={t("teamSaving")}
              onClick={() => void save()}
            >
              {t("save")}
            </Button>
          ) : null}
        </div>
      </div>
      <div className={styles.body}>
        <TextArea
          ref={editorRef}
          className={styles.editor}
          aria-label={t("teamGuideTab")}
          aria-describedby={hintID}
          rows={4}
          value={value}
          placeholder={t("teamGuidePlaceholder")}
          disabled={disabled}
          onChange={(event) => {
            onChange(event.currentTarget.value);
            syncMention(event.currentTarget);
            setSaveFailed(false);
          }}
          onSelect={(event) => syncMention(event.currentTarget)}
          onKeyDown={handleKeyDown}
          onBlur={() => setMention(null)}
          onCompositionStart={() => {
            composing.current = true;
            setMention(null);
          }}
          onCompositionEnd={(event) => {
            composing.current = false;
            syncMention(event.currentTarget);
          }}
        />
        {!disabled && candidates.length ? (
          <MentionPicker
            className={styles.mentions}
            users={candidates}
            activeIndex={activeIndex}
            onSelect={applyMention}
            t={t}
          />
        ) : null}
      </div>
      <p className={styles.hint} id={hintID} title={t("teamGuideHint")}>
        <AtSign size={13} aria-hidden="true" />
        {t("teamGuideMentionHint", { count: members.length })}
      </p>
      {saveFailed ? (
        <p className={styles.error} role="alert">
          {t("teamActionFailed")}
        </p>
      ) : null}
    </section>
  );
}

import { useLayoutEffect, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import type { MentionPickerUser } from "@/components/business/ConversationPane";
import { getMentionCandidates } from "@/models/composer";
import { serializeTaskPromptMentions, updateTaskPromptMentions, type TaskPromptMention } from "./taskPromptMentions";

export function useTaskPromptMentions({
  value,
  onChange,
  members,
  editorRef,
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  members: MentionPickerUser[];
  editorRef: RefObject<HTMLTextAreaElement | null>;
  disabled: boolean;
}) {
  const [mention, setMention] = useState<{ start: number; end: number; query: string } | null>(null);
  const [chosen, setChosen] = useState<TaskPromptMention[]>([]);
  const [activeIndex, setActiveIndex] = useState(0);
  const pendingCaret = useRef<number | null>(null);
  const composing = useRef(false);
  const candidates =
    mention && !disabled
      ? (getMentionCandidates(members, mention.query, { limit: members.length }) as MentionPickerUser[])
      : [];
  useLayoutEffect(() => {
    if (pendingCaret.current === null) return;
    editorRef.current?.focus();
    editorRef.current?.setSelectionRange(pendingCaret.current, pendingCaret.current);
    pendingCaret.current = null;
  }, [value, editorRef]);

  function sync(editor: HTMLTextAreaElement) {
    const match =
      !disabled && !composing.current && editor.selectionStart === editor.selectionEnd
        ? /(?:^|\s)@([^\s@]*)$/u.exec(editor.value.slice(0, editor.selectionStart))
        : null;
    const next = match
      ? { start: editor.selectionStart - match[1].length - 1, end: editor.selectionStart, query: match[1] }
      : null;
    if (next?.start === mention?.start && next?.end === mention?.end && next?.query === mention?.query) return;
    setMention(next);
    setActiveIndex(0);
  }

  function change(editor: HTMLTextAreaElement) {
    const next = editor.value;
    setChosen((current) => updateTaskPromptMentions(value, next, current));
    onChange(next);
    sync(editor);
  }

  function choose(member: MentionPickerUser) {
    if (!mention || disabled) return;
    const name = member.name || member.id;
    const text = `@${name}`;
    const next = value.slice(0, mention.start) + text + " " + value.slice(mention.end);
    setChosen((current) => [
      ...updateTaskPromptMentions(value, next, current),
      { start: mention.start, end: mention.start + text.length, userId: member.id, userName: name },
    ]);
    pendingCaret.current = mention.start + text.length + 1;
    onChange(next);
    setMention(null);
  }

  function keyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (composing.current || event.nativeEvent.isComposing || event.keyCode === 229 || !mention) return false;
    if (event.key === "Escape") {
      event.preventDefault();
      setMention(null);
      return true;
    }
    if (!candidates.length) return false;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex((index) => (index + (event.key === "ArrowDown" ? 1 : -1) + candidates.length) % candidates.length);
      return true;
    }
    if ((event.key === "Enter" || event.key === "Tab") && !event.shiftKey) {
      event.preventDefault();
      choose(candidates[activeIndex] || candidates[0]);
      return true;
    }
    return false;
  }

  return {
    candidates,
    activeIndex,
    choose,
    change,
    sync,
    keyDown,
    dismiss: () => setMention(null),
    clear: () => {
      setMention(null);
      setChosen([]);
    },
    compositionStart: () => {
      composing.current = true;
      setMention(null);
    },
    compositionEnd: (editor: HTMLTextAreaElement) => {
      composing.current = false;
      sync(editor);
    },
    serialize: (text: string) =>
      serializeTaskPromptMentions(text, text === value ? chosen : [], new Set(members.map((member) => member.id))),
  };
}

import { useLayoutEffect, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import type { MentionPickerUser } from "@/components/business/ConversationPane";
import { useQuery } from "@tanstack/react-query";
import { fetchWorkspaceDirectory } from "@/api/chatFiles";
import { fileReferenceKey, type ComposerFileReference } from "@/models/fileReferences";
import { getMentionCandidates, splitTextSegmentByMentions } from "@/models/composer";
import {
  serializeTaskPromptMentions,
  taskPromptFileReferences,
  updateTaskPromptMentions,
  withoutTaskPromptFiles,
  type TaskPromptMention,
} from "./taskPromptMentions";

export function useTaskPromptMentions({
  value,
  onChange,
  members,
  editorRef,
  workspaceAgentID,
  workspaceDir,
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  members: MentionPickerUser[];
  editorRef: RefObject<HTMLTextAreaElement | null>;
  disabled: boolean;
  workspaceAgentID: string;
  workspaceDir?: string;
}) {
  const [mention, setMention] = useState<{ start: number; end: number; query: string } | null>(null);
  const [chosen, setChosen] = useState<TaskPromptMention[]>([]);
  const [activeIndex, setActiveIndex] = useState(0);
  const [directory, setDirectory] = useState("");
  const pendingCaret = useRef<number | null>(null);
  const latestDraft = useRef({ value, chosen });
  const composing = useRef(false);
  const listing = useQuery({
    queryKey: ["chat-workspace", "", workspaceAgentID, directory, workspaceDir],
    queryFn: ({ signal }) => fetchWorkspaceDirectory(workspaceAgentID, directory, signal, "", workspaceDir),
    enabled: Boolean(mention && !disabled && workspaceAgentID),
  });
  const scopedMentions = chosen.filter(
    (item) =>
      !("reference" in item) || (item.reference.source === "workspace" && item.reference.agent_id === workspaceAgentID),
  );
  const candidates: MentionPickerUser[] =
    mention && !disabled
      ? [
          ...(!directory ? getMentionCandidates(members, mention.query, { limit: members.length }) : []),
          ...(workspaceAgentID
            ? (listing.data?.entries ?? [])
                .filter((entry) =>
                  (entry.name || entry.path).toLocaleLowerCase().includes(mention.query.toLocaleLowerCase()),
                )
                .slice(0, 20)
                .map((entry) => {
                  const fileReference: ComposerFileReference = {
                    source: "workspace",
                    agent_id: workspaceAgentID,
                    path: entry.path,
                    label: entry.path + (entry.type === "dir" ? "/" : ""),
                    name: entry.name || entry.path,
                  };
                  return { id: fileReferenceKey(fileReference), name: fileReference.name, fileReference };
                })
            : []),
        ]
      : [];
  useLayoutEffect(() => {
    latestDraft.current = { value, chosen };
  }, [value, chosen]);
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
    if (!next || next.start !== mention?.start) setDirectory("");
    setMention(next);
    setActiveIndex(0);
  }

  function change(editor: HTMLTextAreaElement) {
    const next = editor.value;
    setChosen((current) => updateTaskPromptMentions(value, next, current));
    onChange(next);
    sync(editor);
  }

  function pasteText(editor: HTMLTextAreaElement, text: string) {
    const names = new Map(
      members.flatMap((member) => (member.name ? [[member.name.toLowerCase(), member] as const] : [])),
    );
    const start = editor.selectionStart;
    const end = editor.selectionEnd;
    let pasted = "";
    const inserted: TaskPromptMention[] = [];
    for (const segment of splitTextSegmentByMentions(text, names)) {
      if (segment.type === "mention") {
        const mentionStart = start + pasted.length;
        pasted += `@${segment.userName}`;
        inserted.push({ ...segment, start: mentionStart, end: start + pasted.length });
      } else if (segment.type === "text") {
        pasted += segment.text;
      }
    }
    editor.setRangeText(pasted, start, end, "end");
    const next = editor.value;
    const shift = pasted.length - (end - start);
    setChosen((current) => [
      ...current.flatMap((item) => {
        if (item.end <= start) return [item];
        if (item.start >= end) return [{ ...item, start: item.start + shift, end: item.end + shift }];
        return [];
      }),
      ...inserted,
    ]);
    pendingCaret.current = editor.selectionStart;
    onChange(next);
    sync(editor);
  }

  function insert(
    selection: { start: number; end: number },
    item: { reference: ComposerFileReference } | { userId: string; userName: string },
  ) {
    const name = "reference" in item ? item.reference.label : item.userName;
    const prefix = selection.start > 0 && !/\s/u.test(value[selection.start - 1]) ? " " : "";
    const text = `@${name}`;
    const next = value.slice(0, selection.start) + prefix + text + " " + value.slice(selection.end);
    const start = selection.start + prefix.length;
    setChosen((current) => [
      ...updateTaskPromptMentions(value, next, current),
      { ...item, start, end: start + text.length },
    ]);
    pendingCaret.current = start + text.length + 1;
    onChange(next);
    setMention(null);
    setDirectory("");
  }

  function choose(member: MentionPickerUser) {
    if (!mention || disabled) return;
    insert(
      mention,
      member.fileReference
        ? { reference: member.fileReference }
        : { userId: member.id, userName: member.name || member.id },
    );
  }

  function referenceFile(reference: ComposerFileReference) {
    const editor = editorRef.current;
    if (!editor || disabled || reference.source !== "workspace" || reference.agent_id !== workspaceAgentID) return;
    insert({ start: editor.selectionStart, end: editor.selectionEnd }, { reference });
  }

  function clearFiles(clearPeople = false) {
    const draft = latestDraft.current;
    const next = withoutTaskPromptFiles(draft.value, draft.chosen);
    setMention(null);
    setDirectory("");
    setChosen(clearPeople ? [] : next.mentions);
    if (next.text !== draft.value) onChange(next.text);
  }

  function keyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (composing.current || event.nativeEvent.isComposing || event.keyCode === 229) return false;
    const editor = event.currentTarget;
    if (editor.selectionStart === editor.selectionEnd && (event.key === "Backspace" || event.key === "Delete")) {
      const caret = editor.selectionStart;
      const adjacent = scopedMentions.find((item) =>
        event.key === "Backspace" ? item.end === caret : item.start === caret,
      );
      if (adjacent) {
        event.preventDefault();
        editor.setRangeText("", adjacent.start, adjacent.end, "end");
        pendingCaret.current = adjacent.start;
        change(editor);
        return true;
      }
    }
    if (!mention) return false;
    if (event.key === "ArrowLeft" && directory && !mention.query) {
      event.preventDefault();
      setDirectory(directory.split("/").slice(0, -1).join("/"));
      setActiveIndex(0);
      return true;
    }
    if (event.key === "Escape") {
      event.preventDefault();
      setMention(null);
      setDirectory("");
      return true;
    }
    if (!candidates.length) {
      if (!directory || event.key !== "Enter" || event.shiftKey) return false;
      event.preventDefault();
      return true;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex((index) => (index + (event.key === "ArrowDown" ? 1 : -1) + candidates.length) % candidates.length);
      return true;
    }
    if ((event.key === "Enter" || event.key === "Tab") && !event.shiftKey) {
      event.preventDefault();
      const candidate = candidates[activeIndex] || candidates[0];
      const file = candidate.fileReference;
      if (event.key === "Tab" && file?.source === "workspace" && file.label.endsWith("/")) {
        editor.setRangeText("@", mention.start, mention.end, "end");
        pendingCaret.current = mention.start + 1;
        change(editor);
        setDirectory(file.path.replace(/\/$/u, ""));
        setActiveIndex(0);
      } else {
        choose(candidate);
      }
      return true;
    }
    return false;
  }

  return {
    candidates,
    mentionRanges: scopedMentions
      .filter((item) => {
        const label = "reference" in item ? item.reference.label : item.userName;
        return value.slice(item.start, item.end) === `@${label}`;
      })
      .sort((a, b) => a.start - b.start),
    activeIndex,
    choose,
    referenceFile,
    clearFiles,
    fileReferences: (text: string) => taskPromptFileReferences(text, text === value ? scopedMentions : []),
    change,
    pasteText,
    sync,
    keyDown,
    dismiss: () => {
      setMention(null);
      setDirectory("");
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
      serializeTaskPromptMentions(
        text,
        text === value ? scopedMentions : [],
        new Set(members.map((member) => member.id)),
      ),
  };
}

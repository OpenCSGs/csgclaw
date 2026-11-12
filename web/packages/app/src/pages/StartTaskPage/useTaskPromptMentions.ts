import { useLayoutEffect, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import type { MentionPickerUser } from "@/components/business/ConversationPane";
import { useQuery } from "@tanstack/react-query";
import { fetchWorkspaceDirectory } from "@/api/chatFiles";
import { fileReferenceKey, type ComposerFileReference } from "@/models/fileReferences";
import { getMentionCandidates } from "@/models/composer";
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
  const pendingCaret = useRef<number | null>(null);
  const latestDraft = useRef({ value, chosen });
  const composing = useRef(false);
  const listing = useQuery({
    queryKey: ["chat-workspace", "", workspaceAgentID, "", workspaceDir],
    queryFn: ({ signal }) => fetchWorkspaceDirectory(workspaceAgentID, "", signal, "", workspaceDir),
    enabled: Boolean(mention && !disabled && workspaceAgentID),
  });
  const scopedMentions = chosen.filter(
    (item) =>
      !("reference" in item) || (item.reference.source === "workspace" && item.reference.agent_id === workspaceAgentID),
  );
  const candidates: MentionPickerUser[] =
    mention && !disabled
      ? [
          ...getMentionCandidates(members, mention.query, { limit: members.length }),
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
    setMention(next);
    setActiveIndex(0);
  }

  function change(editor: HTMLTextAreaElement) {
    const next = editor.value;
    setChosen((current) => updateTaskPromptMentions(value, next, current));
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
    setChosen(clearPeople ? [] : next.mentions);
    if (next.text !== draft.value) onChange(next.text);
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
    fileMentions: scopedMentions
      .filter((item) => "reference" in item && value.slice(item.start, item.end) === `@${item.reference.label}`)
      .sort((a, b) => a.start - b.start),
    activeIndex,
    choose,
    referenceFile,
    clearFiles,
    fileReferences: (text: string) => taskPromptFileReferences(text, text === value ? scopedMentions : []),
    change,
    sync,
    keyDown,
    dismiss: () => setMention(null),
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

import { useRoomFileReader } from "@/components/business/ChatSidePanel/useRoomFileReader";
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import { File, Folder, User } from "lucide-react";
import { fetchWorkspaceDirectory, type RoomFile } from "@/api/chatFiles";
import {
  getComposerMentionState,
  insertComposerSegmentsAtSelection,
  isComposerKeyboardEventComposing,
  type ComposerMentionState,
} from "@/models/composer";
import type { ComposerFileReference } from "@/models/fileReferences";
import type { WorkspaceEntry } from "@/models/workspace";
import type { TranslateFn } from "@/models/conversations";
import type { ChatFileScope } from "@/components/business/ChatSidePanel/types";
import { useChatSidePanel } from "@/components/business/ChatSidePanel/useChatSidePanel";
import type { MentionPickerUser } from "../types";

type Candidate =
  | { kind: "person"; user: MentionPickerUser }
  | { kind: "file"; reference: ComposerFileReference; sourceLabel: string }
  | { kind: "directory"; path: string; reference: ComposerFileReference };

export function useFileMentions({
  scope,
  editorRef,
  onSync,
  people,
  personIndex = 0,
  onPerson,
  t,
}: {
  scope?: ChatFileScope;
  editorRef: RefObject<HTMLDivElement | null>;
  onSync: () => void;
  people: MentionPickerUser[];
  personIndex?: number;
  onPerson: (user: MentionPickerUser) => void;
  t: TranslateFn;
}) {
  const { state } = useChatSidePanel(scope?.userID ?? "", scope?.roomID ?? "");
  const [query, setQuery] = useState<ComposerMentionState | null>(null);
  const [entries, setEntries] = useState<WorkspaceEntry[]>([]);
  const [attachments, setAttachments] = useState<RoomFile[]>([]);
  const [index, setIndex] = useState(0);
  const [dismissed, setDismissed] = useState<string | null>(null);
  const agentID = scope?.agents.some((agent) => agent.id === state.agentID)
    ? state.agentID
    : (scope?.defaultAgentID ?? "");
  const slash = query?.query.lastIndexOf("/") ?? -1;
  const directory = slash >= 0 ? query!.query.slice(0, slash).replace(/^\/+/, "") : state.directory;
  const filter = (slash >= 0 ? query!.query.slice(slash + 1) : (query?.query ?? "")).toLowerCase();
  const queryText = query?.query;
  const roomID = scope?.roomID;
  const { read: readFiles } = useRoomFileReader(scope);
  const hasQuery = queryText !== undefined;
  useEffect(() => {
    if (!roomID || !hasQuery || !agentID) return;
    const abort = new AbortController();
    setEntries([]);
    void fetchWorkspaceDirectory(agentID, directory, abort.signal, roomID)
      .then((listing) => setEntries(listing.entries ?? []))
      .catch(() => {});
    return () => abort.abort();
  }, [roomID, hasQuery, agentID, directory]);
  useEffect(() => {
    if (!roomID || queryText === undefined) return;
    const abort = new AbortController();
    setAttachments([]);
    const timer = setTimeout(() => {
      void readFiles(queryText, 0, abort.signal)
        .then((files) => setAttachments(files.items))
        .catch(() => {});
    }, 150);
    return () => {
      clearTimeout(timer);
      abort.abort();
    };
  }, [roomID, queryText, readFiles]);
  const candidates = useMemo<Candidate[]>(
    () => [
      ...people.map((user) => ({ kind: "person" as const, user })),
      ...entries
        .filter((entry) => (entry.name ?? entry.path).toLowerCase().includes(filter))
        .slice(0, 20)
        .map((entry) =>
          entry.type === "dir"
            ? {
                kind: "directory" as const,
                path: entry.path,
                reference: {
                  source: "workspace" as const,
                  agent_id: agentID,
                  path: entry.path,
                  name: entry.name ?? entry.path,
                  label: entry.path + "/",
                },
              }
            : {
                kind: "file" as const,
                reference: {
                  source: "workspace" as const,
                  agent_id: agentID,
                  path: entry.path,
                  name: entry.name ?? entry.path,
                  label: entry.path,
                },
                sourceLabel: scope?.agents.find((agent) => agent.id === agentID)?.name ?? agentID,
              },
        ),
      ...attachments.slice(0, 20).map((file) => ({
        kind: "file" as const,
        reference: { source: "attachment" as const, attachment_id: file.id, name: file.name, label: file.name },
        sourceLabel: t("chatConversationFiles"),
      })),
    ],
    [people, entries, attachments, filter, agentID, scope?.agents, t],
  );
  function sync() {
    if (!scope) return;
    const next = getComposerMentionState(editorRef.current);
    setQuery((current) =>
      current?.query === next?.query &&
      current?.startOffset === next?.startOffset &&
      current?.endOffset === next?.endOffset
        ? current
        : next,
    );
    if (next?.query !== query?.query) {
      setIndex(0);
      setDismissed(null);
    }
  }
  const activeIndex = query ? index : personIndex;
  const activeRef = useRef<HTMLButtonElement | null>(null);
  useLayoutEffect(() => {
    activeRef.current?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, candidates.length]);
  const open = Boolean(scope && (query || people.length) && dismissed !== query?.query && candidates.length);
  function apply(candidate: Candidate, navigate = false) {
    if (candidate.kind === "person") {
      onPerson(candidate.user);
      setQuery(null);
      return;
    }
    const editor = editorRef.current;
    const mention = getComposerMentionState(editor);
    if (!editor || !mention?.textNode) return;
    const selection = window.getSelection();
    const range = document.createRange();
    range.setStart(mention.textNode, mention.startOffset);
    range.setEnd(mention.textNode, mention.endOffset);
    selection?.removeAllRanges();
    selection?.addRange(range);
    const segments =
      candidate.kind === "directory" && navigate
        ? [{ type: "text" as const, text: `@${candidate.path}/` }]
        : [
            { type: "file" as const, reference: candidate.reference, text: `@${candidate.reference.label}` },
            { type: "text" as const, text: " " },
          ];
    insertComposerSegmentsAtSelection(segments);
    onSync();
    sync();
    setIndex(0);
    editor.focus();
  }
  function handleKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (!open || isComposerKeyboardEventComposing(event)) return false;
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      setDismissed(query?.query ?? "");
      return true;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setIndex((value) => (value + (event.key === "ArrowDown" ? 1 : -1) + candidates.length) % candidates.length);
      return true;
    }
    if ((event.key === "Enter" && !event.shiftKey) || event.key === "Tab") {
      event.preventDefault();
      const candidate = candidates[Math.min(index, candidates.length - 1)];
      if (candidate) apply(candidate, event.key === "Tab");
      return true;
    }
    return false;
  }
  const picker = open ? (
    <div className="mention-picker" role="listbox" aria-label={t("chatFileReference")}>
      {candidates.map((candidate, i) => {
        const person = candidate.kind === "person";
        const label = person
          ? (candidate.user.name ?? candidate.user.id)
          : candidate.kind === "directory"
            ? candidate.path + "/"
            : candidate.reference.label;
        const Icon = person ? User : candidate.kind === "directory" ? Folder : File;
        return (
          <div
            key={
              person
                ? `person:${candidate.user.id}`
                : candidate.kind === "directory"
                  ? candidate.path
                  : `file:${candidate.reference.source}:${candidate.reference.label}:${i}`
            }
          >
            {i === 0 || (candidates[i - 1].kind === "person" && !person) ? (
              <div className="chat-file-mention-group">{t(person ? "chatMentionPeople" : "chatMentionFiles")}</div>
            ) : null}
            <button
              type="button"
              role="option"
              aria-selected={i === activeIndex}
              ref={i === activeIndex ? activeRef : undefined}
              className={`mention-option${i === activeIndex ? " active" : ""}`}
              onMouseDown={(event) => {
                event.preventDefault();
                apply(candidate);
              }}
            >
              <span className="chat-file-mention-icon">
                <Icon size={16} />
              </span>
              <span>
                <span className="message-author">@{label}</span>
                <span className="conversation-preview">
                  {person
                    ? candidate.user.id
                    : candidate.kind === "file"
                      ? candidate.sourceLabel
                      : t("chatDirectoryCompletionHint")}
                </span>
              </span>
            </button>
          </div>
        );
      })}
    </div>
  ) : null;
  return { picker, sync, handleKeyDown };
}

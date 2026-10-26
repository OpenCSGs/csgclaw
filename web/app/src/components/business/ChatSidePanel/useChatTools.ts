import { createElement, useCallback, useEffect, useMemo, useRef } from "react";
import type { RefObject } from "react";
import type { AgentLike } from "@/models/agents";
import { isManagerAgent, isNotificationBotAgent } from "@/models/agents";
import type { IMConversation, TranslateFn, UsersById } from "@/models/conversations";
import { isDirectConversation } from "@/models/conversations";
import { insertComposerSegmentsAtSelection, placeCaretAtEnd } from "@/models/composer";
import { fileReferenceKey, referenceForPreview, type ComposerFileReference } from "@/models/fileReferences";
import { conversationActivityAgents } from "@/components/business/ConversationActivityPanel/conversationActivity";
import type { DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import type { CitationSelection } from "@/components/business/MessageContent";
import { parseCitationDocument } from "@/models/citations";
import { ChatToolsToolbar } from "./ChatToolsToolbar";
import { useChatSidePanel } from "./useChatSidePanel";

type ChatToolsOptions = {
  conversation: IMConversation;
  currentUserID: string;
  agents: AgentLike[];
  usersById: UsersById;
  editorRef: RefObject<HTMLDivElement | null>;
  t: TranslateFn;
  hasRoomTasks?: boolean;
  onOpenTool?: () => void;
  suspended?: boolean;
};

export function useChatTools({
  conversation,
  currentUserID,
  agents,
  editorRef,
  t,
  hasRoomTasks,
  onOpenTool,
  suspended,
}: ChatToolsOptions) {
  const controller = useChatSidePanel(currentUserID, conversation.id);
  const direct = isDirectConversation(conversation);
  const triggerRef = useRef<HTMLElement | null>(null);
  const selectionRef = useRef<{ editor: HTMLElement; range: Range } | null>(null);
  const fileAgents = useMemo(
    () =>
      conversationActivityAgents(
        conversation,
        agents.filter((agent) => !isNotificationBotAgent(agent)),
      ),
    [conversation, agents],
  );
  const defaultAgentID = direct
    ? (fileAgents[0]?.id ?? "")
    : (fileAgents.find(
        (agent) =>
          agent.identities.includes(conversation.manager_id ?? "") ||
          isManagerAgent(agents.find((candidate) => candidate.id === agent.id)),
      )?.id ??
      fileAgents[0]?.id ??
      "");
  const fileScope = useMemo(
    () => ({
      roomID: conversation.id,
      direct,
      sharedWorkspace: !direct && Boolean(conversation.configuration?.workspace_dir),
      userID: currentUserID,
      agents: fileAgents.map(({ id, name }) => ({ id, name })),
      defaultAgentID,
      retainedMessages: conversation.threads?.flatMap((thread) => thread.context ?? []),
      messages: conversation.messages,
    }),
    [
      conversation.configuration?.workspace_dir,
      conversation.id,
      conversation.threads,
      conversation.messages,
      currentUserID,
      fileAgents,
      defaultAgentID,
      direct,
    ],
  );
  const { openTab, patch } = controller;
  useEffect(() => {
    selectionRef.current = null;
    const remember = () => {
      const selection = window.getSelection();
      if (!selection?.rangeCount) return;
      const range = selection.getRangeAt(0);
      const element =
        range.startContainer instanceof Element ? range.startContainer : range.startContainer.parentElement;
      const editor = element?.closest<HTMLElement>('[contenteditable="true"]');
      const chat = editorRef.current?.closest(".chat-panel");
      if (editor && chat?.contains(editor) && !editor.closest(".chat-side-panel")?.hasAttribute("hidden"))
        selectionRef.current = { editor, range: range.cloneRange() };
    };
    document.addEventListener("selectionchange", remember);
    return () => document.removeEventListener("selectionchange", remember);
  }, [editorRef, conversation.id]);

  const onReference = useCallback(
    (reference: ComposerFileReference) => {
      const saved = selectionRef.current;
      const editor = saved?.editor.isConnected ? saved.editor : editorRef.current;
      if (!editor || editor.getAttribute("contenteditable") === "false") return;
      const sideTab = editor.closest<HTMLElement>("[data-side-tab-id]");
      if (sideTab?.dataset.sideTabId) patch({ activeID: sideTab.dataset.sideTabId, open: true });
      else if (editor.closest(".chat-panel")?.querySelector(".chat-side-panel")?.matches(".is-overlay, .is-maximized"))
        patch({ open: false });
      requestAnimationFrame(() => {
        editor.focus({ preventScroll: true });
        if (saved && editor.contains(saved.range.startContainer)) {
          const selection = window.getSelection();
          selection?.removeAllRanges();
          selection?.addRange(saved.range);
        } else placeCaretAtEnd(editor);
        insertComposerSegmentsAtSelection([
          { type: "file", reference, text: `@${reference.label}` },
          { type: "text", text: " " },
        ]);
        editor.dispatchEvent(new Event("input", { bubbles: true }));
      });
    },
    [editorRef, patch],
  );

  const openPreview = useCallback(
    (request: DocumentPreviewRequest, originTabID?: "files") => {
      const item = request.items[request.index] ?? request.items[0];
      if (!item) return;
      triggerRef.current = request.anchor ?? null;
      const reference = referenceForPreview(item);
      openTab({
        id: reference ? fileReferenceKey(reference) : `draft:${item.id}`,
        kind: "file",
        name: item.name,
        sizeBytes: item.sizeBytes,
        mediaType: item.mediaType,
        reference,
        draft: item.file ? item : undefined,
        originTabID,
      });
    },
    [openTab],
  );
  const openActivity = useCallback(
    (entryID?: string) => openTab({ id: "activity", kind: "activity", entryID }),
    [openTab],
  );
  const openSources = useCallback(
    (selection: CitationSelection) => {
      triggerRef.current = selection.anchor;
      const messageID =
        conversation.messages.find((message) => {
          const sources = parseCitationDocument(message.content ?? "").sources;
          return selection.cited.every((citation) => sources.get(citation.id)?.snippet === citation.snippet);
        })?.id ?? "";
      openTab({ id: "sources", kind: "sources", activeID: selection.activeID, messageID, cited: selection.cited });
    },
    [conversation.messages, openTab],
  );
  const collapse = useCallback(() => {
    patch({ open: false });
    requestAnimationFrame(() => triggerRef.current?.isConnected && triggerRef.current.focus({ preventScroll: true }));
  }, [patch]);
  const toolbar = createElement(ChatToolsToolbar, {
    controller,
    onActivity: openActivity,
    hasRoomTasks,
    onOpenTool,
    suspended,
    triggerRef,
    t,
  });

  return { controller, fileScope, onReference, openPreview, openActivity, openSources, collapse, toolbar };
}

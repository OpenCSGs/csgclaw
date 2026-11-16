import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { Conversation, useConversationDraftEditorSync } from "@/components/business/ConversationPane";
import chatPanelStyles from "@/components/business/ConversationPane/ChatPanel.module.css";
import {
  isComposerKeyboardEventComposing,
  parseComposerSegments,
  segmentsToPlainText,
  type ComposerSegment,
} from "@/models/composer";
import { buildUsersById, type IMConversation, type IMMessage } from "@/models/conversations";
import { createTranslator } from "@/shared/i18n";
import type { ResolvedThemeMode } from "@/shared/theme/theme";
import { useLabTranslation } from "../LabLocale";
import styles from "./MessagingDemo.module.css";

const selfID = "ui-lab-you";
const peerID = "ui-lab-phoenix";
const roomID = "ui-lab-messaging";
const noop = () => {};

export function MessagingDemo({ theme }: { theme: ResolvedThemeMode }) {
  const { locale } = useLabTranslation();
  const t = createTranslator(locale);
  const zh = locale === "zh";
  const peer = { id: peerID, name: "Phoenix Baker", is_online: true };
  const members = [{ id: selfID, name: zh ? "我" : "You" }, peer];
  const usersById = buildUsersById(members);
  const [startedAt] = useState(() => Date.now());
  const [messages, setMessages] = useState<IMMessage[]>([]);
  const [draft, setDraft] = useState<ComposerSegment[]>([]);
  const [threadID, setThreadID] = useState<string>();
  const [threadDraft, setThreadDraft] = useState<ComposerSegment[]>([]);
  const [replies, setReplies] = useState<Record<string, IMMessage[]>>({});
  const editorRef = useRef<HTMLDivElement>(null);
  const messageListRef = useRef<HTMLElement>(null);
  const channelToolsRef = useRef<HTMLDivElement>(null);
  const nextID = useRef(0);

  const visibleMessages: IMMessage[] = [
    {
      id: "ui-lab-greeting",
      sender_id: peerID,
      created_at: new Date(startedAt - 90_000).toISOString(),
      content: zh ? "你好！可以帮我看看这版设计吗？" : "Hi! Could you take a look at this design?",
    },
    {
      id: "ui-lab-answer",
      sender_id: selfID,
      created_at: new Date(startedAt - 60_000).toISOString(),
      content: zh ? "可以，我今天会看一下。" : "Sure, I’ll take a look today.",
    },
    ...messages,
  ].map((message) => {
    const count = replies[message.id ?? ""]?.length ?? 0;
    return count ? { ...message, thread: { root_id: message.id, reply_count: count } } : message;
  });
  const conversation: IMConversation = {
    id: roomID,
    is_direct: true,
    members: [selfID, peerID],
    messages: visibleMessages,
  };
  const root = visibleMessages.find((message) => message.id === threadID);
  useConversationDraftEditorSync(editorRef, draft, t("remove"));

  useLayoutEffect(() => {
    const list = messageListRef.current;
    if (list) list.scrollTop = list.scrollHeight;
  }, [messages.length]);

  const syncDraft = useCallback(() => {
    if (editorRef.current) setDraft(parseComposerSegments(editorRef.current));
  }, []);

  function localMessage(segments: ComposerSegment[]): IMMessage | undefined {
    const content = segmentsToPlainText(segments).trim();
    if (!content) return;
    return {
      id: `ui-lab-message-${++nextID.current}`,
      sender_id: selfID,
      content,
      created_at: new Date().toISOString(),
    };
  }

  function send() {
    const editor = editorRef.current;
    if (!editor) return;
    const message = localMessage(parseComposerSegments(editor));
    if (!message) return;
    setMessages((current) => [...current, message]);
    setDraft([]);
    editor.focus();
  }

  return (
    <div className={styles.preview} data-thread-open={Boolean(root)}>
      <div className={`chat-panel ${chatPanelStyles["chat-panel"]} ${styles.window}`}>
        <Conversation.Header
          channelToolsRef={channelToolsRef}
          conversation={conversation}
          conversationMembers={members}
          currentUserID={selfID}
          description={zh ? "本地对话示例" : "Local conversation demo"}
          inviteActionLabel=""
          selectedMessageCount={visibleMessages.length}
          showChannelTools={false}
          showChannelToolsAction={false}
          showInviteAction={false}
          showMemberListAction={false}
          showToolCalls={false}
          onClearMessages={noop}
          onDeleteRoom={noop}
          onInviteAction={noop}
          onPreviewUser={noop}
          onToggleChannelTools={noop}
          onToggleToolCalls={noop}
          t={t}
          usersById={usersById}
        />
        <Conversation.MessageList
          conversation={conversation}
          currentUserID={selfID}
          locale={locale}
          messageActionBusy=""
          messageActionFeedback={{}}
          messageListRef={messageListRef}
          onMessageAction={noop}
          onOpenThread={(message) => {
            setThreadID(message.id);
            setThreadDraft([]);
          }}
          onPreviewUser={noop}
          t={t}
          theme={theme}
          usersById={usersById}
          visibleMessages={visibleMessages}
        />
        <Conversation.Composer
          authBusyProvider=""
          authStatuses={{}}
          composerDisabled={false}
          composerError=""
          draftSegments={draft}
          draftText={segmentsToPlainText(draft)}
          editorRef={editorRef}
          managerProvider=""
          mentionCandidates={[]}
          mentionIndex={0}
          mentionableUsersByName={new Map()}
          onApplyMention={noop}
          onApplySlashCandidate={noop}
          onComposerCompositionStart={noop}
          onComposerCompositionEnd={syncDraft}
          onComposerKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey && !isComposerKeyboardEventComposing(event)) {
              event.preventDefault();
              send();
            }
          }}
          onProviderLogin={noop}
          onSendMessage={send}
          onSyncComposer={syncDraft}
          slashCandidates={[]}
          slashIndex={0}
          slashPickerLoading={false}
          slashPickerOpen={false}
          t={t}
        />
      </div>
      {root && threadID && (
        <Conversation.ThreadPanel
          currentUserID={selfID}
          disabled={false}
          draftSegments={threadDraft}
          error=""
          loading={false}
          locale={locale}
          onClose={() => setThreadID(undefined)}
          onDraftChange={setThreadDraft}
          onPreviewUser={noop}
          onSend={() => {
            const message = localMessage(threadDraft);
            if (!message) return;
            setReplies((current) => ({ ...current, [threadID]: [...(current[threadID] ?? []), message] }));
            setThreadDraft([]);
          }}
          showToolCalls={false}
          t={t}
          theme={theme}
          thread={{ room_id: roomID, root, replies: replies[threadID] ?? [], summary: root.thread }}
          usersById={usersById}
        />
      )}
    </div>
  );
}

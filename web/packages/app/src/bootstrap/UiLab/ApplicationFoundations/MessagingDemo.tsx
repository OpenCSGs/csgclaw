import { useCallback, useLayoutEffect, useRef, useState } from "react";
import type { UserInputResponse } from "@/api/agentActivities";
import {
  AgentQuestionComposer,
  Conversation,
  useConversationDraftEditorSync,
  useQuestionAnswerMode,
} from "@/components/business/ConversationPane";
import { DocumentPreviewPanel, type DocumentPreviewRequest } from "@/components/business/DocumentPreviewPanel";
import { AgentActivityMsgTypes } from "@/shared/constants/messages";
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
import { generatedImageExample } from "../generatedImageExample";
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
  const [preview, setPreview] = useState<DocumentPreviewRequest>();
  const [questionOpen, setQuestionOpen] = useState(false);
  const [answer, setAnswer] = useState<string>();
  const questionID = "ui-lab-media-question";
  const question = {
    id: "style",
    header: zh ? "头像风格" : "Avatar style",
    question: zh ? "下一张头像要用哪种风格？" : "What style should the next avatar use?",
    is_other: true,
    options: [{ label: zh ? "卡通" : "Cartoon" }, { label: zh ? "写实" : "Realistic" }],
  };
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
    generatedImageExample(zh, new Date(startedAt - 30_000).toISOString(), peerID),
    {
      id: questionID,
      sender_id: peerID,
      created_at: new Date(startedAt - 15_000).toISOString(),
      content:
        answer === undefined ? question.question : zh ? `已收到你的回答：${answer}` : `Answer received: ${answer}`,
      metadata: {
        csgclaw: {
          agent_activity: {
            type: "com.opencsg.csgclaw.agent.activity",
            version: 1,
            event_id: questionID,
            sender: peerID,
            channel: "csgclaw",
            room_id: roomID,
            origin_server_ts: startedAt - 15_000,
            content: {
              msgtype: AgentActivityMsgTypes.question,
              body: question.question,
              question: {
                id: questionID,
                status: answer === undefined ? "pending" : "answered",
                questions: [question],
              },
            },
          },
        },
      },
    },
    ...messages,
  ].map((message) => {
    const count = replies[message.id ?? ""]?.length ?? 0;
    return count ? { ...message, thread: { root_id: message.id, reply_count: count } } : message;
  });
  const respond = useCallback(
    async (_channel: string, id: string, response: UserInputResponse) => {
      setAnswer(response.answers.style?.answers[0]?.replace(/^user_note: /, "") || t("questionSkipped"));
      setQuestionOpen(false);
      return { id, status: "answered", questions: [] };
    },
    [t],
  );
  const questionMode = useQuestionAnswerMode({ messages: visibleMessages, responderID: selfID, roomID, respond, t });
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
  }, [messages.length, questionOpen, answer]);

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
          onPreviewAttachment={setPreview}
          onQuestionSelect={(id, itemID, optionIndex) => {
            questionMode.select(id, itemID, optionIndex);
            setQuestionOpen(true);
          }}
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
        {questionOpen && questionMode.selected ? (
          <AgentQuestionComposer mode={questionMode} t={t} usersById={usersById} />
        ) : (
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
        )}
      </div>
      {preview ? (
        <DocumentPreviewPanel
          {...preview}
          onClose={() => setPreview(undefined)}
          onIndexChange={(index) => setPreview((current) => current && { ...current, index })}
          t={t}
        />
      ) : null}
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

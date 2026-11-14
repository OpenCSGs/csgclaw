import conversationSendButtonStyles from "@/components/business/ConversationPane/ConversationSendButton.module.css";
import { CloudPrivacyDialog } from "@/components/business/CloudPrivacyDialog";
import { useCloudSendConsent } from "@/hooks/workspace/useCloudSendConsent";
import { useCloudAgents } from "@/hooks/workspace/useCloudAgents";
import { createUUID } from "@/shared/lib/uuid";
import { ResourceTag } from "@/components/business/ResourceTag";
import {
  effectiveConversationProfile,
  supportsTaskConfiguration,
  type TaskConfiguration,
} from "@/models/taskConfiguration";
import { useId, useMemo, useRef, useState } from "react";
import { useSearchParams, useLocation } from "react-router-dom";
import {
  ArrowUp,
  BookOpen,
  ClipboardCheck,
  FileText,
  PanelRightOpen,
  Presentation,
  Search,
  RotateCcw,
  Square,
  Undo2,
  Users,
  X,
} from "lucide-react";
import { Button, DismissibleInlineError, PopoverAnchor, PopoverContent, PopoverRoot, Tooltip } from "@/components/ui";
import { ChatSidePanel, useChatSidePanel } from "@/components/business/ChatSidePanel";
import { closeChatSideTab } from "@/models/chatSidePanel";
import { fileReferenceKey } from "@/models/fileReferences";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { sendMessageRequest } from "@/api/im";
import { errorMessage } from "@/api/client";
import { localizeAPIError } from "@/shared/i18n";
import { normalizeAuthProviderName, providerNeedsAuth, resolveAgentChannelUserID } from "@/models/agents";
import { modelProviderConfigUsesOpenCSG } from "@/models/modelProviders";
import { taskConversationTitle } from "@/models/taskConversations";
import type { IMConversation } from "@/models/conversations";
import styles from "./StartTaskPage.module.css";
import { TargetPicker, SOLO_TEAM_ID } from "./components/TargetPicker";
import {
  MentionPicker,
  SlashPicker,
  handleSlashPickerNavigation,
  type MentionPickerUser,
} from "@/components/business/ConversationPane";
import { AttachmentDraftStrip } from "@/components/business/ConversationPane/ConversationAttachments";
import { dataTransferHasFiles, filesFromDataTransfer } from "@/components/business/ConversationPane/attachmentFiles";
import { isComposerKeyboardEventComposing } from "@/models/composer";
import {
  attachmentSelectionError,
  createAttachmentDrafts,
  selectAttachmentFiles,
  type AttachmentDraft,
} from "@/models/attachments";
import {
  builtinSlashCommandNames,
  composerActionSuggestions,
  normalizeSlashShorthandForPayload,
  suggestedSlashCommandNames,
} from "@/models/slashCommands";
import {
  buildSlashPickerState,
  slashCommandInputText,
  slashPickerQueryForDraft,
} from "@/hooks/workspace/useConversationController";
import { useInfiniteAgentSkills } from "@/hooks/workspace/useInfiniteAgentSkills";
import { useSlashSelection } from "@/hooks/workspace/useSlashSelection";
import { useTaskPromptMentions } from "./useTaskPromptMentions";
import { TaskPromptEditor } from "./components/TaskPromptEditor";
import {
  ConversationResourcePicker,
  useComposerResourcePicker,
} from "@/components/business/ConversationResourcePicker";
import { ComposerAddMenu } from "@/components/business/ConversationPane/ComposerAddMenu";
import {
  resourceMessageInput,
  resourceSegment,
  canSendResources,
  uniqueResources,
  resourceKey,
  type ResourceSnapshot,
} from "@/models/conversationResources";
import { TaskConfigurationToolbar } from "@/components/business/TaskConfigurationToolbar";
import { useWorkspaceDialogs } from "@/hooks/workspace/useWorkspaceDialogs";
import { TASK_EXAMPLES_HIDDEN_STORAGE_KEY } from "@/shared/storage/keys";
import "@/components/business/ConversationPane/ConversationPane.css";

export function StartTaskPage() {
  const location = useLocation();
  const [params] = useSearchParams();
  const initialTarget = params.get("cloud")
    ? `cloud:${params.get("cloud")}`
    : params.get("team")
      ? `team:${params.get("team")}`
      : params.get("agent")
        ? `agent:${params.get("agent")}`
        : "";
  return (
    <StartTaskForm
      key={`${location.pathname}:${initialTarget}`}
      initialTarget={initialTarget}
      initialConfiguration={location.state?.configuration}
    />
  );
}

function StartTaskForm({
  initialTarget,
  initialConfiguration,
}: {
  initialTarget: string;
  initialConfiguration?: TaskConfiguration;
}) {
  const controller = useWorkspaceControllerContext();
  const cloud = useCloudAgents(controller.sidebarProps?.authStatus);
  const { openSettings } = useWorkspaceDialogs();
  const [configuration, setConfiguration] = useState<TaskConfiguration>(initialConfiguration || {});
  const [{ target, revision }, setTarget] = useState({ target: initialTarget, revision: 0 });
  const [resources, setResources] = useState<ResourceSnapshot[]>([]);
  const [examplesHidden, setExamplesHidden] = useState(() => {
    try {
      return sessionStorage.getItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY) === "true";
    } catch {
      return false;
    }
  });
  const [draft, setDraft] = useState("");
  const [attachments, setAttachments] = useState<AttachmentDraft[]>([]);
  const [removedAttachments, setRemovedAttachments] = useState<{ draft: AttachmentDraft; index: number }[]>([]);
  const [attachmentWarning, setAttachmentWarning] = useState("");
  const [uploadProgress, setUploadProgress] = useState<number | null>(null);
  const [uploadFailed, setUploadFailed] = useState(false);
  const [slashQuery, setSlashQuery] = useState<string | null>(null);
  const [slashIndex, setSlashIndex] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const created = useRef<IMConversation | null>(null);
  const requestID = useRef(createUUID());
  const targetRevision = useRef(0);
  const sendID = useRef({ fingerprint: "", id: "" });
  const inFlight = useRef(false);
  const composing = useRef(false);
  const upload = useRef<AbortController | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const pageRef = useRef<HTMLElement>(null);
  const panelTrigger = useRef<HTMLButtonElement>(null);
  const panelID = useId();
  const sidebar = controller.ready ? controller.sidebarProps : null;
  const sidePanel = useChatSidePanel(sidebar?.currentUserID ?? "", `start-task-${panelID}`, false);
  const agents = useMemo(() => sidebar?.workerAgentItems ?? [], [sidebar?.workerAgentItems]);
  const teams = useMemo(() => sidebar?.teams ?? [], [sidebar?.teams]);
  const manager = agents.find((agent) => agent.role === "manager");
  const selected = target || `agent:${manager?.id || agents[0]?.id || ""}`;
  const consent = useCloudSendConsent(selected);
  const selectedCloud = cloud.agents.find((agent) => selected === `cloud:${agent.id}`);
  const selectedAgent = agents.find((agent) => selected === `agent:${agent.id}`);
  const selectedTeam = teams.find((team) => selected === `team:${team.id}`);
  const cloudTarget = selected.startsWith("cloud:");
  const workspaceDir =
    !cloudTarget && (!selectedAgent || supportsTaskConfiguration(selectedAgent))
      ? configuration.workspace_dir
      : undefined;
  const automaticWorkspace = !cloudTarget && !selectedAgent && !workspaceDir;
  const workspaceAgent = cloudTarget || automaticWorkspace ? undefined : (selectedAgent ?? manager);
  const workspaceAgentID = workspaceAgent?.id || "";
  const mentionMembers = useMemo(() => {
    const ids = new Set(selectedTeam ? [selectedTeam.lead_agent_id, ...selectedTeam.member_agent_ids] : []);
    return agents
      .filter((agent) =>
        selectedAgent
          ? agent.id === selectedAgent.id
          : selectedTeam
            ? agent === manager || ids.has(agent.id || "")
            : selected === "solo" && agent === manager,
      )
      .map(
        (agent): MentionPickerUser => ({
          id: resolveAgentChannelUserID(agent) || agent.id || "",
          name: agent.name || agent.user_name || agent.id,
          avatar: agent.avatar,
          role: agent.role,
        }),
      );
  }, [agents, selectedAgent, selectedTeam, selected, manager]);
  const resourceAgentID = supportsTaskConfiguration(selectedAgent) ? selectedAgent?.id || undefined : undefined;
  const resourcePicker = useComposerResourcePicker({
    resourceAgentID,
    onPickResources: (items) => setResources((current) => uniqueResources([...current, ...items])),
    editorRef,
    selected: resources,
    disabled: busy,
  });
  const promptMentions = useTaskPromptMentions({
    value: draft,
    onChange: setDraft,
    members: mentionMembers,
    editorRef,
    workspaceAgentID,
    workspaceDir,
    disabled: busy || resourcePicker.active,
  });
  const slashAgentID = cloudTarget ? "" : (selectedAgent || manager)?.id || "";
  const slashEnabled = Boolean(slashAgentID && !busy && !resourcePicker.active);
  const skills = useInfiniteAgentSkills(slashAgentID, slashQuery, slashEnabled, Boolean(resourceAgentID));
  const slashState = useMemo(
    () =>
      buildSlashPickerState({ draftText: draft, enabled: slashEnabled, query: slashQuery, skillOptions: skills.items }),
    [draft, slashEnabled, slashQuery, skills.items],
  );
  useSlashSelection(slashState.candidates, slashIndex, setSlashIndex, JSON.stringify([slashAgentID, slashQuery]));
  if (!controller.ready || !sidebar || !controller.taskEntry) return null;
  const readySidebar = sidebar;
  const { t } = sidebar;
  const entry = controller.taskEntry;
  const recipient = selectedCloud ? undefined : selectedAgent || manager;
  const selectedProfile = recipient
    ? effectiveConversationProfile(recipient, selectedAgent ? configuration : undefined, controller.modelProviders)
    : null;
  const modelMissing = recipient && (selectedProfile?.profile_complete === false || !selectedProfile?.model_id);
  const targetName = selectedCloud?.name || selectedAgent?.name || selectedTeam?.title || t("taskEntrySolo");

  function resetFilePanel(nextCloud = cloudTarget, clearDrafts = false) {
    sidePanel.patch((current) => {
      const next = current.tabs.reduce(
        (state, tab) =>
          (tab.kind === "file" && (!tab.draft || clearDrafts)) || (nextCloud && tab.kind === "files")
            ? closeChatSideTab(state, tab.id)
            : state,
        current,
      );
      return { tabs: next.tabs, activeID: next.activeID, directory: "", agentID: "" };
    });
  }

  function messageInput(prompt: string) {
    if (selectedCloud) return { content: prompt.trim(), resource_refs: [] };
    const content = promptMentions.serialize(prompt).trim();
    return supportsTaskConfiguration(selectedAgent)
      ? resourceMessageInput(
          resources.map((resource) => resourceSegment(resource)),
          content,
        )
      : { content: normalizeSlashShorthandForPayload(content), resource_refs: [] };
  }
  const actionSuggestions = cloudTarget || resourcePicker.active ? [] : composerActionSuggestions(draft);
  const composerHelpId = `${panelID}-composer-help`;
  const draftInput = messageInput(draft);
  const draftFileCount = promptMentions.fileReferences(draft).length;
  const cannotSend =
    busy ||
    Boolean(selectedCloud && selectedCloud.status !== "online") ||
    resourcePicker.active ||
    !canSendResources(draftInput.content, draftInput.resource_refs, attachments.length + draftFileCount) ||
    Boolean(modelMissing);
  const sendLabel = t(uploadProgress !== null ? "stopSending" : "taskEntryStart");

  function syncSlash(editor: HTMLTextAreaElement) {
    const query =
      !composing.current && editor.selectionStart === editor.selectionEnd
        ? slashPickerQueryForDraft(editor.value.slice(0, editor.selectionStart))
        : null;
    if (query === slashQuery) return;
    setSlashQuery(query);
    setSlashIndex(0);
  }

  function applySlashCandidate(name: string) {
    const editor = editorRef.current;
    if (!editor || !name) return;
    const start = editor.value.length - editor.value.trimStart().length;
    const end = editor.value.trimStart().startsWith("/")
      ? start + (editor.value.slice(start).match(/^\S+/u)?.[0].length ?? 0)
      : editor.value.length;
    const isCommand = builtinSlashCommandNames.includes(name) || suggestedSlashCommandNames.includes(name);
    if (!isCommand && resourceAgentID) {
      setResources((current) => uniqueResources([...current, { source: "skill", id: name, name, type: "skill" }]));
      editor.setRangeText("", 0, end, "end");
    } else {
      editor.setRangeText(
        slashCommandInputText(name).trimEnd() + (end === editor.value.length ? " " : ""),
        start,
        end,
        "end",
      );
    }
    promptMentions.change(editor);
    setSlashQuery(null);
    editor.focus();
  }

  function addAttachments(files: File[]) {
    if (busy || cloudTarget || !files.length) return;
    const selection = selectAttachmentFiles(files, attachments);
    setAttachmentWarning(attachmentSelectionError(selection, t));
    if (!selection.files.length) return;
    setAttachments([...attachments, ...createAttachmentDrafts(selection.files, attachments.length)]);
    setRemovedAttachments([]);
    setUploadFailed(false);
  }

  function removeAttachment(id: string) {
    const index = attachments.findIndex((item) => item.id === id);
    if (busy || index < 0) return;
    setAttachments(attachments.filter((item) => item.id !== id));
    setRemovedAttachments([...removedAttachments, { draft: attachments[index], index }]);
    setAttachmentWarning("");
    setUploadFailed(false);
  }

  function undoRemoveAttachments() {
    const next = [...attachments];
    for (const { draft, index } of [...removedAttachments].reverse()) next.splice(index, 0, draft);
    setAttachments(next);
    setRemovedAttachments([]);
  }

  async function start(empty = false, prompt = draft, confirmed = false) {
    const input = messageInput(prompt);
    const fileReferences = selectedCloud ? [] : promptMentions.fileReferences(prompt);
    if (
      inFlight.current ||
      (!empty && !canSendResources(input.content, input.resource_refs, attachments.length + fileReferences.length))
    )
      return;
    if (!selectedCloud && !selectedAgent && !selectedTeam && selected !== "solo") {
      setError(t("taskEntryMissingTarget"));
      return;
    }
    if (selectedCloud && selectedCloud.status !== "online") {
      setError(t("cloudChatOffline"));
      return;
    }
    if (!empty && selectedCloud && !confirmed && consent.request(() => void start(false, prompt, true))) return;
    inFlight.current = true;
    setBusy(true);
    setError("");
    try {
      if (modelMissing) throw new Error(t("taskEntryModelMissing"));
      if (!empty) {
        if (modelProviderConfigUsesOpenCSG(selectedProfile) && !readySidebar.onRequireOpenCSGAuthentication?.()) return;
        const provider = normalizeAuthProviderName(selectedProfile?.provider);
        if (
          providerNeedsAuth(provider) &&
          controller.agentViewProps?.authStatuses?.[provider]?.authenticated === false
        ) {
          throw new Error(t("authRequired"));
        }
      }
      let room = created.current;
      if (!room) {
        const agentIDs = selectedTeam ? [selectedTeam.lead_agent_id, ...selectedTeam.member_agent_ids] : [];
        const members = selectedCloud
          ? []
          : selectedAgent
            ? [resolveAgentChannelUserID(selectedAgent) || selectedAgent.id || ""]
            : Array.from(
                new Set([
                  resolveAgentChannelUserID(manager) || manager?.id || "",
                  ...agentIDs.map((id) => resolveAgentChannelUserID(agents.find((item) => item.id === id)) || id),
                ]),
              ).filter(Boolean);
        if (!selectedCloud && !selectedAgent && !manager) throw new Error(t("taskEntryManagerMissing"));
        if (selectedTeam && agentIDs.some((id) => !agents.some((agent) => agent.id === id))) {
          throw new Error(t("taskEntryMembersMissing"));
        }
        room = await entry.create({
          auto_title: Boolean(selectedAgent && !selectedCloud),
          title: taskConversationTitle(prompt, `${targetName} · ${new Date().toLocaleString(readySidebar.locale)}`),
          member_ids: members,
          cloud_agent: selectedCloud
            ? { agent_id: selectedCloud.id, connection_id: selectedCloud.connectionID }
            : undefined,
          type: selectedAgent || selectedCloud ? "free" : "on_demand",
          is_direct: Boolean(selectedAgent || selectedCloud),
          configuration: selectedCloud
            ? undefined
            : selectedAgent
              ? supportsTaskConfiguration(selectedAgent)
                ? configuration
                : undefined
              : { workspace_dir: configuration.workspace_dir },
          source_team_id: selectedTeam?.id,
          manager_id:
            selectedAgent || selectedCloud ? undefined : resolveAgentChannelUserID(manager) || manager?.id || undefined,
          client_request_id: requestID.current,
          locale: readySidebar.locale,
        });
        created.current = room;
        if (room.configuration) {
          if (room.configuration.workspace_dir !== workspaceDir) resetFilePanel();
          setConfiguration(room.configuration);
        }
      }
      if (!empty) {
        const fingerprint = JSON.stringify([
          input.content,
          input.resource_refs,
          fileReferences,
          attachments.map(({ file }) => [file.name, file.size, file.lastModified]),
        ]);
        if (sendID.current.fingerprint !== fingerprint) sendID.current = { fingerprint, id: createUUID() };
        const messageFiles = selectedCloud ? [] : attachments.map(({ file }) => file);
        const payload = {
          room_id: room.id,
          sender_id: readySidebar.currentUserID,
          content: input.content,
          resource_refs: input.resource_refs,
          ...(fileReferences.length ? { file_references: fileReferences } : {}),
          attachments: messageFiles,
          client_message_id: sendID.current.id,
          locale: readySidebar.locale,
        };
        if (messageFiles.length) {
          upload.current = new AbortController();
          setUploadProgress(0);
          setUploadFailed(false);
        }
        const message = upload.current
          ? await sendMessageRequest(payload, { signal: upload.current.signal, onUploadProgress: setUploadProgress })
          : await sendMessageRequest(payload);
        entry.onMessage(room.id, message);
      }
      entry.open(room);
    } catch (cause) {
      if (!upload.current?.signal.aborted) {
        setUploadFailed(Boolean(upload.current));
        setError(localizeAPIError(cause, t, errorMessage(cause, t("sendFailed"))));
      }
    } finally {
      upload.current = null;
      setUploadProgress(null);
      inFlight.current = false;
      setBusy(false);
    }
  }

  const examples =
    selectedAgent || selectedCloud
      ? [
          { key: "knowledgeCard", icon: BookOpen },
          { key: "meetingNotes", icon: FileText },
          { key: "aiTrends", icon: Presentation },
        ]
      : [
          { key: "members", icon: Users },
          { key: "research", icon: Search },
          { key: "review", icon: ClipboardCheck },
        ];
  return (
    <section className={styles.page} ref={pageRef}>
      {selectedCloud && consent.dialog ? (
        <CloudPrivacyDialog agentName={selectedCloud.name} confirmation={consent.dialog} t={t} />
      ) : null}
      <div className={styles.main}>
        {!sidePanel.state.open ? (
          <Tooltip content={t("chatSideExpand")}>
            <Button
              ref={panelTrigger}
              className={styles.panelToggle}
              iconOnly
              size="sm"
              variant="tertiaryGray"
              aria-label={t("chatSideExpand")}
              onClick={() => sidePanel.patch({ open: true })}
            >
              <PanelRightOpen size={17} />
            </Button>
          </Tooltip>
        ) : null}
        <div className={styles.inner}>
          <header className={styles.head}>
            <h1>{t("iaStartTitle")}</h1>
            <p>{t(selectedAgent || selectedCloud ? "iaStartSubtitleDirect" : "iaStartSubtitleRooms")}</p>
          </header>
          <div className={styles.pane}>
            <TargetPicker
              agents={agents}
              cloudAgents={cloud.agents}
              teams={teams}
              usersById={sidebar.usersById}
              currentUserID={sidebar.currentUserID}
              value={
                selected === "solo"
                  ? { kind: "team", id: SOLO_TEAM_ID }
                  : {
                      kind: selected.startsWith("cloud:") ? "cloud" : selected.startsWith("team:") ? "team" : "agent",
                      id: selected.slice(selected.indexOf(":") + 1),
                    }
              }
              onChange={(value) => {
                targetRevision.current++;
                requestID.current = createUUID();
                promptMentions.clearFiles(true);
                resetFilePanel(value.kind === "cloud", true);
                setResources([]);
                setAttachments([]);
                setRemovedAttachments([]);
                setAttachmentWarning("");
                setUploadFailed(false);
                setSlashQuery(null);
                setTarget({
                  target: value.id === SOLO_TEAM_ID ? "solo" : `${value.kind}:${value.id}`,
                  revision: targetRevision.current,
                });
                setConfiguration({});
              }}
              locked={busy || Boolean(created.current)}
              t={t}
              onCreateAgent={() => void sidebar.onCreateCustomAgent()}
              onCreateTeam={() => void sidebar.onOpenCreateTeam()}
            />
            {!selectedCloud && (modelMissing || !agents.length) ? (
              <div className={styles.setup} role="status">
                <strong>{t("iaStartNeedsModelTitle")}</strong>
                <p>{t(modelMissing ? "taskEntryModelMissing" : "taskEntryNoAgents")}</p>
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() =>
                    selectedAgent ? void controller.openAgentSettings(selectedAgent) : openSettings("models")
                  }
                >
                  {t("iaStartNeedsModelAction")}
                </Button>
              </div>
            ) : null}
            <div className={styles.composerGroup}>
              <div
                className={styles.composer}
                onDragOver={(event) => {
                  if (!dataTransferHasFiles(event.dataTransfer)) return;
                  event.preventDefault();
                  event.dataTransfer.dropEffect = busy || cloudTarget ? "none" : "copy";
                }}
                onDrop={(event) => {
                  const dropped = filesFromDataTransfer(event.dataTransfer);
                  if (!dropped.length) return;
                  event.preventDefault();
                  addAttachments(dropped);
                }}
              >
                <AttachmentDraftStrip
                  drafts={attachments}
                  progress={uploadProgress ?? 0}
                  status={uploadProgress !== null ? "uploading" : uploadFailed ? "failed" : "idle"}
                  t={t}
                  onRemove={removeAttachment}
                  onPreviewAttachment={({ items, index }) => {
                    const item = items[index];
                    if (!item) return;
                    sidePanel.openTab({
                      id: `draft:${item.id}`,
                      kind: "file",
                      name: item.name,
                      sizeBytes: item.sizeBytes,
                      mediaType: item.mediaType,
                      draft: item,
                    });
                  }}
                />
                {resourcePicker.pickerProps ? (
                  <ConversationResourcePicker
                    key={`${resourceAgentID}:${resourcePicker.pickerProps.kind}`}
                    {...resourcePicker.pickerProps}
                    t={t}
                    className={styles.resourceSearch}
                  />
                ) : null}
                <div className={styles.draft} hidden={resourcePicker.active}>
                  {resources.map((resource) => (
                    <ResourceTag
                      key={resourceKey(resource)}
                      resource={resource}
                      removal={{
                        label: t("remove"),
                        disabled: busy,
                        onRemove: () =>
                          setResources((current) =>
                            current.filter((item) => resourceKey(item) !== resourceKey(resource)),
                          ),
                      }}
                    />
                  ))}
                  <TaskPromptEditor
                    ref={editorRef}
                    autoFocus
                    mentionRanges={promptMentions.mentionRanges}
                    aria-label={t("taskEntryPrompt")}
                    aria-describedby={composerHelpId}
                    placeholder={
                      resources.length || attachments.length || draft.length
                        ? ""
                        : t(selectedCloud ? "cloudChatPlaceholder" : "iaStartComposerPlaceholder")
                    }
                    value={draft}
                    onChange={(event) => {
                      promptMentions.change(event.currentTarget);
                      syncSlash(event.currentTarget);
                    }}
                    onSelect={(event) => {
                      promptMentions.sync(event.currentTarget);
                      syncSlash(event.currentTarget);
                    }}
                    onBlur={() => {
                      promptMentions.dismiss();
                      setSlashQuery(null);
                    }}
                    onCompositionStart={() => {
                      composing.current = true;
                      promptMentions.compositionStart();
                      setSlashQuery(null);
                    }}
                    onCompositionEnd={(event) => {
                      composing.current = false;
                      promptMentions.compositionEnd(event.currentTarget);
                      syncSlash(event.currentTarget);
                    }}
                    onPaste={(event) => {
                      const pasted = filesFromDataTransfer(event.clipboardData);
                      const text = event.clipboardData.getData("text/plain");
                      if (pasted.length && (!cloudTarget || !text)) {
                        event.preventDefault();
                        addAttachments(pasted);
                      }
                      if (text && !cloudTarget) {
                        event.preventDefault();
                        promptMentions.pasteText(event.currentTarget, text);
                        syncSlash(event.currentTarget);
                      }
                    }}
                    disabled={busy}
                    onKeyDown={(event) => {
                      if (composing.current || isComposerKeyboardEventComposing(event) || promptMentions.keyDown(event))
                        return;
                      if (
                        handleSlashPickerNavigation({
                          event,
                          candidates: slashState.candidates,
                          activeIndex: slashIndex,
                          continuation: skills.continuation,
                          pickerOpen: slashState.active,
                          onIndexChange: setSlashIndex,
                          onApply: applySlashCandidate,
                          onDismiss: () => setSlashQuery(null),
                        })
                      )
                        return;
                      if (event.key === "Enter" && !event.shiftKey) {
                        event.preventDefault();
                        void start();
                      }
                    }}
                  />
                </div>
                {promptMentions.candidates.length || promptMentions.directoryStatus || slashState.active ? (
                  <PopoverRoot
                    open
                    onOpenChange={(open) => {
                      if (!open) {
                        promptMentions.dismiss();
                        setSlashQuery(null);
                      }
                    }}
                  >
                    <PopoverAnchor asChild>
                      <span className={styles.mentionAnchor} aria-hidden="true" />
                    </PopoverAnchor>
                    <PopoverContent
                      side="top"
                      align="start"
                      role="presentation"
                      className={`${styles.mentionPopover} ${
                        promptMentions.candidates.length || promptMentions.directoryStatus ? "" : styles.slashPopover
                      }`}
                      onOpenAutoFocus={(event) => event.preventDefault()}
                      onCloseAutoFocus={(event) => event.preventDefault()}
                      onInteractOutside={(event) => {
                        if (event.target === editorRef.current) event.preventDefault();
                      }}
                    >
                      {promptMentions.candidates.length || promptMentions.directoryStatus ? (
                        <>
                          {promptMentions.candidates.length ? (
                            <MentionPicker
                              groupPeopleAndFiles
                              showRole={false}
                              className={`${styles.mentions} ${styles.peopleAndFiles}`}
                              users={promptMentions.candidates}
                              activeIndex={promptMentions.activeIndex}
                              onSelect={promptMentions.choose}
                              t={t}
                            />
                          ) : null}
                          {promptMentions.directoryStatus ? (
                            <div className={styles.mentionStatus}>
                              <div role="status">
                                {promptMentions.directoryStatus.path ? (
                                  <div>{promptMentions.directoryStatus.path}/</div>
                                ) : null}
                                {t(promptMentions.directoryStatus.message)}
                              </div>
                              {promptMentions.directoryStatus.retry ? (
                                <Button size="sm" variant="tertiaryGray" onClick={promptMentions.directoryStatus.retry}>
                                  {t("retry")}
                                </Button>
                              ) : null}
                            </div>
                          ) : null}
                        </>
                      ) : (
                        <SlashPicker
                          className={styles.mentions}
                          candidates={slashState.candidates}
                          activeIndex={slashIndex}
                          continuation={skills.continuation}
                          loading={skills.loading}
                          onSelect={applySlashCandidate}
                          t={t}
                        />
                      )}
                    </PopoverContent>
                  </PopoverRoot>
                ) : null}
                {actionSuggestions.length ? (
                  <div className="flex flex-wrap items-center gap-2" aria-label={t("suggestedActions")}>
                    <span>{t("suggestedActions")}</span>
                    {actionSuggestions.map((suggestion) => (
                      <Button
                        key={suggestion.name}
                        size="sm"
                        variant="secondaryGray"
                        disabled={busy}
                        onClick={() => applySlashCandidate(suggestion.name)}
                      >
                        /{suggestion.name}
                      </Button>
                    ))}
                    <small>{t("suggestedActionsOnly")}</small>
                  </div>
                ) : null}
                <span id={composerHelpId} className="sr-only">
                  {t("composerTip")}
                </span>
                <div className={selectedCloud ? `${styles.actions} ${styles.cloudActions}` : styles.actions}>
                  {!selectedCloud ? (
                    <>
                      <input
                        type="file"
                        multiple
                        ref={fileInput}
                        className="sr-only"
                        aria-label={t("addAttachment")}
                        disabled={busy}
                        onChange={(event) => {
                          addAttachments(Array.from(event.currentTarget.files ?? []));
                          event.currentTarget.value = "";
                        }}
                      />
                      <ComposerAddMenu
                        onOpenResourcePicker={resourcePicker.open}
                        disabled={busy}
                        t={t}
                        buttonClassName={`composer-add-button ${styles.addButton}`}
                        onAddFiles={() => fileInput.current?.click()}
                      />
                      <TaskConfigurationToolbar
                        configuration={configuration}
                        onChange={async (value) => {
                          if (targetRevision.current !== revision || inFlight.current) return;
                          if (created.current) await entry.patch(created.current.id, { configuration: value });
                          else requestID.current = createUUID();
                          if (value.workspace_dir !== configuration.workspace_dir) {
                            promptMentions.clearFiles();
                            resetFilePanel();
                          }
                          setConfiguration(value);
                        }}
                        directoryLocked={Boolean(created.current)}
                        directoryMode={selectedAgent ? "agent" : "collaboration"}
                        agent={selectedAgent || manager}
                        showModel={Boolean(selectedAgent)}
                        showDirectory
                        disabled={busy}
                      />
                    </>
                  ) : null}
                  {busy ? (
                    <span role="status" aria-live="polite">
                      {uploadProgress === null
                        ? t("sending")
                        : t("sendingWithProgress", { progress: Math.round(uploadProgress) })}
                    </span>
                  ) : null}
                  <Tooltip content={sendLabel}>
                    <span>
                      <Button
                        className={`composer-send-button ${conversationSendButtonStyles["composer-send-button"]}`}
                        variant={uploadProgress !== null ? "danger" : "primary"}
                        iconOnly
                        size="sm"
                        aria-label={sendLabel}
                        disabled={uploadProgress === null && cannotSend}
                        onClick={() => (upload.current ? upload.current.abort() : void start())}
                      >
                        {uploadProgress !== null ? (
                          <Square aria-hidden="true" size={14} fill="currentColor" />
                        ) : (
                          <ArrowUp aria-hidden="true" size={17} strokeWidth={1.5} />
                        )}
                      </Button>
                    </span>
                  </Tooltip>
                </div>
              </div>
              {removedAttachments.length ? (
                <div className={styles.attachmentFeedback} role="status">
                  <span>
                    {removedAttachments.length === 1
                      ? t("attachmentRemoved", { name: removedAttachments[0].draft.name })
                      : t("attachmentsRemoved", { count: removedAttachments.length })}
                  </span>
                  <Button size="sm" variant="secondaryGray" disabled={busy} onClick={undoRemoveAttachments}>
                    <Undo2 aria-hidden="true" size={14} />
                    {t("undo")}
                  </Button>
                </div>
              ) : null}
              {attachmentWarning ? (
                <DismissibleInlineError messageKey={attachmentWarning} closeLabel={t("close")}>
                  <span role="alert">{attachmentWarning}</span>
                </DismissibleInlineError>
              ) : null}
            </div>
            {selectedCloud && selectedCloud.status !== "online" ? <p role="status">{t("cloudChatOffline")}</p> : null}
            {error ? (
              <p role="alert" className={styles.error}>
                {error}
                {created.current ? ` ${t("taskEntryRetryCreated")}` : ""}
                {created.current && !cannotSend ? (
                  <Button size="sm" variant="secondaryGray" onClick={() => void start()}>
                    <RotateCcw aria-hidden="true" size={14} />
                    {t("retrySend")}
                  </Button>
                ) : null}
              </p>
            ) : null}
            {!selectedAgent && !selectedCloud ? (
              <div className={styles.emptyRoom}>
                <Button variant="linkGray" size="sm" disabled={busy || !manager} onClick={() => void start(true)}>
                  {t("taskEntryEmptyRoom")}
                </Button>
                <span className={styles.emptyRoomHint}>{t("taskEntryEmptyRoomHint")}</span>
              </div>
            ) : null}
          </div>
          {!examplesHidden && !modelMissing && (agents.length || selectedCloud) ? (
            <div className={styles.examples}>
              <span>{t("iaStartExamples")}</span>
              <div className={styles.chips}>
                {examples.map(({ key, icon: Icon }) => (
                  <button
                    key={key}
                    type="button"
                    disabled={busy}
                    className={styles.chip}
                    onClick={() => {
                      const prompt = t(`iaStartExample_${key}_prompt`);
                      promptMentions.clearFiles(true);
                      setDraft(prompt);
                      void start(false, prompt);
                    }}
                  >
                    <Icon size={15} />
                    <span className="min-w-0">{t(`iaStartExample_${key}_title`)}</span>
                  </button>
                ))}
              </div>
              <button
                type="button"
                className={styles.examplesClose}
                aria-label={t("iaStartExamplesHide")}
                onClick={() => {
                  setExamplesHidden(true);
                  try {
                    sessionStorage.setItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY, "true");
                  } catch {
                    /* Storage may be unavailable. */
                  }
                }}
              >
                <X size={16} />
              </button>
            </div>
          ) : null}
        </div>
      </div>
      <ChatSidePanel
        controller={sidePanel}
        fileScope={{
          roomID: "",
          direct: Boolean(selectedAgent),
          sharedWorkspace: Boolean(workspaceDir && !selectedAgent),
          workspaceDir,
          automaticWorkspace,
          userID: readySidebar.currentUserID,
          agents: workspaceAgentID ? [{ id: workspaceAgentID, name: workspaceAgent?.name || workspaceAgentID }] : [],
          defaultAgentID: workspaceAgentID,
        }}
        agents={agents}
        usersById={readySidebar.usersById}
        locale={readySidebar.locale}
        t={t}
        toolKinds={cloudTarget ? ["browser"] : ["files", "browser"]}
        onPreview={(item) => {
          if (!item.reference) return;
          sidePanel.openTab({
            id: fileReferenceKey(item.reference),
            kind: "file",
            name: item.name,
            sizeBytes: item.sizeBytes,
            mediaType: item.mediaType,
            reference: item.reference,
            originTabID: "files",
          });
        }}
        onReference={(reference) => {
          if (pageRef.current?.querySelector(".chat-side-panel:is(.is-overlay, .is-maximized)")) {
            sidePanel.patch({ open: false });
            requestAnimationFrame(() => editorRef.current?.focus({ preventScroll: true }));
          }
          promptMentions.referenceFile(reference);
        }}
        onCollapse={() => {
          sidePanel.patch({ open: false });
          requestAnimationFrame(() => panelTrigger.current?.focus());
        }}
      />
    </section>
  );
}

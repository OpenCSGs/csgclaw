import { ResourceTag } from "@/components/business/ResourceTag";
import {
  effectiveConversationProfile,
  supportsTaskConfiguration,
  type TaskConfiguration,
} from "@/models/taskConfiguration";
import { useMemo, useRef, useState } from "react";
import { useSearchParams, useLocation } from "react-router-dom";
import { ArrowUp, FileText, FolderGit2, Terminal, Users, X } from "lucide-react";
import { Button } from "@/components/ui";
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
  const initialTarget = params.get("team")
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
  const { openSettings } = useWorkspaceDialogs();
  const [configuration, setConfiguration] = useState<TaskConfiguration>(initialConfiguration || {});
  const [target, setTarget] = useState(initialTarget);
  const [resources, setResources] = useState<ResourceSnapshot[]>([]);
  const [examplesHidden, setExamplesHidden] = useState(() => {
    try {
      return sessionStorage.getItem(TASK_EXAMPLES_HIDDEN_STORAGE_KEY) === "true";
    } catch {
      return false;
    }
  });
  const [draft, setDraft] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const created = useRef<IMConversation | null>(null);
  const requestID = useRef(crypto.randomUUID());
  const sendID = useRef({ fingerprint: "", id: "" });
  const inFlight = useRef(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const sidebar = controller.ready ? controller.sidebarProps : null;
  const agents = useMemo(() => sidebar?.workerAgentItems ?? [], [sidebar?.workerAgentItems]);
  const teams = useMemo(() => sidebar?.teams ?? [], [sidebar?.teams]);
  const manager = agents.find((agent) => agent.role === "manager");
  const selected = target || `agent:${manager?.id || agents[0]?.id || ""}`;
  const selectedAgent = agents.find((agent) => selected === `agent:${agent.id}`);
  const resourceAgentID = supportsTaskConfiguration(selectedAgent) ? selectedAgent?.id || undefined : undefined;
  const resourcePicker = useComposerResourcePicker({
    resourceAgentID,
    onPickResources: (items) => setResources((current) => uniqueResources([...current, ...items])),
    editorRef,
    selected: resources,
    disabled: busy,
  });
  if (!controller.ready || !sidebar || !controller.taskEntry) return null;
  const readySidebar = sidebar;
  const { t } = sidebar;
  const entry = controller.taskEntry;
  const selectedTeam = teams.find((team) => selected === `team:${team.id}`);
  const recipient = selectedAgent || manager;
  const selectedProfile = recipient
    ? effectiveConversationProfile(recipient, selectedAgent ? configuration : undefined, controller.modelProviders)
    : null;
  const modelMissing = recipient && (selectedProfile?.profile_complete === false || !selectedProfile?.model_id);
  const targetName = selectedAgent?.name || selectedTeam?.title || t("taskEntrySolo");

  function messageInput(prompt: string) {
    return supportsTaskConfiguration(selectedAgent)
      ? resourceMessageInput(
          resources.map((resource) => resourceSegment(resource)),
          prompt.trim(),
        )
      : { content: prompt.trim(), resource_refs: [] };
  }
  const draftInput = messageInput(draft);

  async function start(empty = false, prompt = draft) {
    const input = messageInput(prompt);
    if (inFlight.current || (!empty && !canSendResources(input.content, input.resource_refs, files.length))) return;
    if (!selectedAgent && !selectedTeam && selected !== "solo") {
      setError(t("taskEntryMissingTarget"));
      return;
    }
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
        const members = selectedAgent
          ? [resolveAgentChannelUserID(selectedAgent) || selectedAgent.id || ""]
          : Array.from(
              new Set([
                resolveAgentChannelUserID(manager) || manager?.id || "",
                ...agentIDs.map((id) => resolveAgentChannelUserID(agents.find((item) => item.id === id)) || id),
              ]),
            ).filter(Boolean);
        if (!selectedAgent && !manager) throw new Error(t("taskEntryManagerMissing"));
        if (selectedTeam && agentIDs.some((id) => !agents.some((agent) => agent.id === id))) {
          throw new Error(t("taskEntryMembersMissing"));
        }
        room = await entry.create({
          title: taskConversationTitle(prompt, `${targetName} · ${new Date().toLocaleString(readySidebar.locale)}`),
          member_ids: members,
          type: selectedAgent ? "free" : "on_demand",
          is_direct: Boolean(selectedAgent),
          configuration: supportsTaskConfiguration(selectedAgent) ? configuration : undefined,
          manager_id: selectedAgent ? undefined : resolveAgentChannelUserID(manager) || manager?.id || undefined,
          client_request_id: requestID.current,
          locale: readySidebar.locale,
        });
        created.current = room;
        if (room.configuration) setConfiguration(room.configuration);
      }
      if (!empty) {
        const fingerprint = JSON.stringify([
          input.content,
          input.resource_refs,
          files.map((file) => [file.name, file.size, file.lastModified]),
        ]);
        if (sendID.current.fingerprint !== fingerprint) sendID.current = { fingerprint, id: crypto.randomUUID() };
        const message = await sendMessageRequest({
          room_id: room.id,
          sender_id: readySidebar.currentUserID,
          content: input.content,
          resource_refs: input.resource_refs,
          attachments: files,
          client_message_id: sendID.current.id,
          locale: readySidebar.locale,
        });
        entry.onMessage(room.id, message);
      }
      entry.open(room);
    } catch (cause) {
      setError(localizeAPIError(cause, t, errorMessage(cause, t("sendFailed"))));
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }

  const examples = selectedAgent
    ? [
        { key: "read", icon: FolderGit2 },
        { key: "run", icon: Terminal },
        { key: "document", icon: FileText },
      ]
    : [
        { key: "review", icon: Users },
        { key: "research", icon: FolderGit2 },
        { key: "plan", icon: FileText },
      ];
  return (
    <section className={styles.page}>
      <div className={styles.inner}>
        <header className={styles.head}>
          <h1>{t("iaStartTitle")}</h1>
          <p>{t(selectedAgent ? "iaStartSubtitleDirect" : "iaStartSubtitleRooms")}</p>
        </header>
        <div className={styles.pane}>
          <TargetPicker
            agents={agents}
            teams={teams}
            usersById={sidebar.usersById}
            value={
              selected === "solo"
                ? { kind: "team", id: SOLO_TEAM_ID }
                : {
                    kind: selected.startsWith("team:") ? "team" : "agent",
                    id: selected.slice(selected.indexOf(":") + 1),
                  }
            }
            onChange={(value) => {
              setResources([]);
              setTarget(value.id === SOLO_TEAM_ID ? "solo" : `${value.kind}:${value.id}`);
              setConfiguration({});
            }}
            locked={busy || Boolean(created.current)}
            t={t}
            onCreateAgent={() => void sidebar.onCreateCustomAgent()}
            onCreateTeam={() => void sidebar.onOpenCreateTeam()}
          />
          {modelMissing || !agents.length ? (
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
            <div className={styles.composer}>
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
                <textarea
                  ref={editorRef}
                  autoFocus
                  className={styles.input}
                  aria-label={t("taskEntryPrompt")}
                  placeholder={t("iaStartComposerPlaceholder")}
                  value={draft}
                  onChange={(event) => setDraft(event.target.value)}
                  disabled={busy}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
                      event.preventDefault();
                      void start();
                    }
                  }}
                />
              </div>
              {files.length ? (
                <ul className={styles.files}>
                  {files.map((file, index) => (
                    <li key={`${file.name}-${index}`}>
                      <span>{file.name}</span>
                      <Button
                        variant="tertiaryGray"
                        size="sm"
                        disabled={busy}
                        aria-label={`${t("taskEntryRemove")} ${file.name}`}
                        onClick={() => setFiles((value) => value.filter((_, at) => at !== index))}
                      >
                        <X size={14} />
                      </Button>
                    </li>
                  ))}
                </ul>
              ) : null}
              <div className={styles.actions}>
                <input
                  type="file"
                  multiple
                  ref={fileInput}
                  hidden
                  onChange={(event) => {
                    setFiles((value) => [...value, ...Array.from(event.target.files ?? [])]);
                    event.target.value = "";
                  }}
                />
                <ComposerAddMenu
                  onOpenResourcePicker={resourcePicker.open}
                  disabled={busy}
                  t={t}
                  onAddFiles={() => fileInput.current?.click()}
                />
                <TaskConfigurationToolbar
                  configuration={configuration}
                  onChange={async (value) => {
                    if (created.current) await entry.patch(created.current.id, { configuration: value });
                    setConfiguration(value);
                  }}
                  directoryLocked={Boolean(created.current)}
                  agent={selectedAgent || manager}
                  showModel={Boolean(selectedAgent)}
                  showDirectory={Boolean(selectedAgent)}
                  disabled={busy}
                />
                <Button
                  className={styles.send}
                  iconOnly
                  size="sm"
                  aria-label={t("taskEntryStart")}
                  disabled={
                    busy ||
                    resourcePicker.active ||
                    !canSendResources(draftInput.content, draftInput.resource_refs, files.length) ||
                    Boolean(modelMissing)
                  }
                  onClick={() => void start()}
                >
                  <ArrowUp size={17} />
                </Button>
              </div>
            </div>
          </div>
          {error ? (
            <p role="alert" className={styles.error}>
              {error}
              {created.current ? ` ${t("taskEntryRetryCreated")}` : ""}
            </p>
          ) : null}
          {!selectedAgent ? (
            <button
              className={styles.emptyRoom}
              type="button"
              disabled={busy || !manager}
              onClick={() => void start(true)}
            >
              {t("taskEntryEmptyRoom")}
            </button>
          ) : null}
        </div>
        {!examplesHidden && !modelMissing && agents.length ? (
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
                    setDraft(prompt);
                    void start(false, prompt);
                  }}
                >
                  <Icon size={15} />
                  {t(`iaStartExample_${key}_title`)}
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
    </section>
  );
}

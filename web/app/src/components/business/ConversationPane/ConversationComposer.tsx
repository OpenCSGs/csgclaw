import { memo, useId, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, RefObject } from "react";
import { ArrowUp, GitBranch, Paperclip, Plus } from "lucide-react";
import { CLIProxyAuthControl } from "@/components/business/ProfileControls";
import { Button, PopoverClose, PopoverContent, PopoverRoot, PopoverTrigger, TextInput, Tooltip } from "@/components/ui";
import { IconImage } from "@/components/ui/Icons";
import type { CLIProxyAuthStatusMap } from "@/hooks/workspace/useCLIProxyAuthStatuses";
import type { AgentProfileLike } from "@/models/agents";
import type { AttachmentDraft } from "@/models/attachments";
import { providerNeedsAuth } from "@/models/agents";
import {
  insertComposerSegmentsAtSelection,
  insertPlainTextAtSelection,
  normalizeTextMentions,
  type ComposerMentionUser,
  type ComposerSegment,
} from "@/models/composer";
import {
  emptyGitHubConnectorStatus,
  emptyGitLabConnectorStatus,
  gitLabConnectorDraftFromStatus,
} from "@/models/connectors";
import type { ConnectorConfigDraft, ConnectorStatus, GitLabConnectorConfigDraft } from "@/models/connectors";
import type { TranslateFn } from "@/models/conversations";
import type { SlashPickerCandidate } from "@/models/slashCommands";
import { MentionPicker } from "./MentionPicker";
import { SlashPicker } from "./SlashPicker";
import { AttachmentDraftStrip } from "./ConversationAttachments";
import { dataTransferHasFiles, filesFromDataTransfer } from "./attachmentFiles";
import {
  ConversationWorkingActions,
  type ConversationWorkingAction,
  type ConversationWorkingParticipant,
  type MentionPickerUser,
  type VoidOrPromise,
} from "./types";

export type ConversationComposerProps = {
  authBusyProvider: string;
  authStatuses: CLIProxyAuthStatusMap;
  connectorBusyAction?: string;
  connectorBusyProvider?: string;
  connectorError?: string;
  connectorPending?: boolean;
  connectorStatus?: ConnectorStatus;
  gitlabConnectorStatus?: ConnectorStatus;
  composerDisabled: boolean;
  composerDisabledReason?: string;
  composerError: string;
  draftSegments: ComposerSegment[];
  draftText: string;
  attachmentDrafts?: AttachmentDraft[];
  editorRef: RefObject<HTMLDivElement | null>;
  managerProfile?: AgentProfileLike | null;
  managerProvider: string;
  mentionCandidates: MentionPickerUser[];
  mentionIndex: number;
  mentionableUsersByName: Map<string, ComposerMentionUser>;
  onApplyMention: (user: MentionPickerUser) => void;
  onApplySlashCandidate: (name: string) => void;
  onAddAttachments?: (files: File[]) => void;
  onComposerCompositionEnd: () => void;
  onComposerCompositionStart: () => void;
  onComposerKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => void;
  onConnectConnector?: () => VoidOrPromise;
  onDisconnectConnector?: () => VoidOrPromise;
  onDisconnectGitLabConnector?: () => VoidOrPromise;
  onManageConnector?: () => VoidOrPromise;
  onProviderLogin: (provider: string) => VoidOrPromise;
  onSaveConnectorConfig?: (draft: ConnectorConfigDraft) => VoidOrPromise;
  onSaveGitLabConnectorConfig?: (draft: GitLabConnectorConfigDraft) => VoidOrPromise;
  onSendMessage: () => VoidOrPromise;
  onRemoveAttachment?: (id: string) => void;
  onSyncComposer: () => void;
  onWorkingAction?: (participant?: ConversationWorkingParticipant) => void;
  slashCandidates: SlashPickerCandidate[];
  slashIndex: number;
  slashPickerLoading: boolean;
  slashPickerOpen: boolean;
  t: TranslateFn;
  workingParticipants?: ConversationWorkingParticipant[];
};

export const ConversationComposer = memo(function ConversationComposer({
  authBusyProvider,
  authStatuses,
  connectorBusyAction = "",
  connectorBusyProvider = "",
  connectorError = "",
  connectorPending = false,
  connectorStatus,
  gitlabConnectorStatus,
  composerDisabled,
  composerDisabledReason = "",
  composerError,
  draftSegments,
  draftText,
  attachmentDrafts = [],
  editorRef,
  managerProfile,
  managerProvider,
  mentionCandidates,
  mentionIndex,
  mentionableUsersByName,
  slashCandidates,
  slashIndex,
  slashPickerLoading,
  slashPickerOpen,
  t,
  workingParticipants = [],
  onApplyMention,
  onApplySlashCandidate,
  onAddAttachments = () => {},
  onComposerCompositionEnd,
  onComposerCompositionStart,
  onComposerKeyDown,
  onConnectConnector,
  onDisconnectConnector,
  onDisconnectGitLabConnector,
  onManageConnector,
  onProviderLogin,
  onSaveGitLabConnectorConfig,
  onRemoveAttachment = () => {},
  onSendMessage,
  onSyncComposer,
  onWorkingAction,
}: ConversationComposerProps) {
  const defaultConnectorStatus = useMemo(() => emptyGitHubConnectorStatus(), []);
  const githubStatus = connectorStatus ?? defaultConnectorStatus;
  const defaultGitLabStatus = useMemo(() => emptyGitLabConnectorStatus(), []);
  const gitlabStatus = gitlabConnectorStatus ?? defaultGitLabStatus;
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const composerHelpId = useId();
  const sendDisabled = composerDisabled || (!draftText.trim() && attachmentDrafts.length === 0);

  function handleFiles(files: File[]) {
    if (composerDisabled || files.length === 0) {
      return;
    }
    onAddAttachments(files);
  }

  return (
    <footer className={`composer${workingParticipants.length > 0 ? " has-working-status" : ""}`}>
      {slashPickerOpen ? (
        <SlashPicker
          candidates={slashCandidates}
          activeIndex={slashIndex}
          loading={slashPickerLoading}
          t={t}
          onSelect={(name) => onApplySlashCandidate(name)}
        />
      ) : null}
      {mentionCandidates.length > 0 ? (
        <MentionPicker users={mentionCandidates} activeIndex={mentionIndex} t={t} onSelect={onApplyMention} />
      ) : null}
      {managerProfile &&
      providerNeedsAuth(managerProfile.provider) &&
      authStatuses[managerProvider]?.authenticated === false ? (
        <CLIProxyAuthControl
          provider={managerProfile.provider}
          t={t}
          status={authStatuses[managerProvider]}
          busy={authBusyProvider === managerProvider}
          onLogin={onProviderLogin}
        />
      ) : null}
      {workingParticipants.length > 0 ? (
        <ComposerWorkingIndicator participants={workingParticipants} t={t} onAction={onWorkingAction} />
      ) : null}
      <div
        className="composer-box"
        onDragOver={(event) => {
          if (composerDisabled || !dataTransferHasFiles(event.dataTransfer)) {
            return;
          }
          event.preventDefault();
          event.dataTransfer.dropEffect = "copy";
        }}
        onDrop={(event) => {
          const files = filesFromDataTransfer(event.dataTransfer);
          if (files.length === 0) {
            return;
          }
          event.preventDefault();
          handleFiles(files);
        }}
      >
        <AttachmentDraftStrip drafts={attachmentDrafts} t={t} onRemove={onRemoveAttachment} />
        <div className="composer-editor-wrap">
          {draftSegments.length === 0 ? (
            <div className="composer-placeholder" aria-hidden="true">
              {composerDisabled ? composerDisabledReason || t("profileIncomplete") : t("inputPlaceholder")}
            </div>
          ) : null}
          <div
            ref={editorRef}
            className={`composer-editor ${composerDisabled ? "disabled" : ""}`}
            contentEditable={composerDisabled ? "false" : "true"}
            suppressContentEditableWarning={true}
            role="textbox"
            aria-multiline="true"
            aria-label={t("inputPlaceholder")}
            aria-describedby={composerHelpId}
            aria-disabled={composerDisabled}
            onInput={onSyncComposer}
            onClick={onSyncComposer}
            onKeyDown={onComposerKeyDown}
            onCompositionStart={onComposerCompositionStart}
            onCompositionEnd={onComposerCompositionEnd}
            onKeyUp={onSyncComposer}
            onPaste={(event) => {
              const files = filesFromDataTransfer(event.clipboardData);
              const pasted = event.clipboardData?.getData("text/plain") ?? "";
              if (files.length > 0) {
                event.preventDefault();
                handleFiles(files);
                if (!pasted) {
                  return;
                }
              } else {
                event.preventDefault();
              }
              const segments = normalizeTextMentions([{ type: "text", text: pasted }], mentionableUsersByName);
              if (segments.some((segment) => segment.type === "mention")) {
                insertComposerSegmentsAtSelection(segments);
              } else {
                insertPlainTextAtSelection(pasted);
              }
              onSyncComposer();
            }}
          />
        </div>
        <div className="composer-toolbar">
          <ComposerAddMenu
            busyAction={connectorBusyAction}
            busyProvider={connectorBusyProvider}
            disabled={composerDisabled}
            error={connectorError}
            pending={connectorPending}
            status={githubStatus}
            gitlabStatus={gitlabStatus}
            t={t}
            onAddFiles={() => fileInputRef.current?.click()}
            onConnect={onConnectConnector}
            onDisconnect={onDisconnectConnector}
            onDisconnectGitLab={onDisconnectGitLabConnector}
            onManage={onManageConnector}
            onSaveGitLab={onSaveGitLabConnectorConfig}
          />
          <input
            ref={fileInputRef}
            className="sr-only"
            type="file"
            multiple
            aria-label={t("addAttachment")}
            onChange={(event) => {
              handleFiles(Array.from(event.currentTarget.files || []));
              event.currentTarget.value = "";
            }}
          />
          <span id={composerHelpId} className="sr-only">
            {t("composerTip")}
          </span>
          <Tooltip content={t("send")}>
            <span>
              <Button
                variant="primary"
                className="composer-send-button"
                aria-label={t("send")}
                disabled={sendDisabled}
                iconOnly
                size="lg"
                onClick={onSendMessage}
              >
                <ArrowUp aria-hidden="true" size={22} strokeWidth={2.25} />
              </Button>
            </span>
          </Tooltip>
        </div>
      </div>
      {composerError ? <div className="form-error composer-error">{composerError}</div> : null}
    </footer>
  );
});

function ComposerWorkingIndicator({
  participants,
  t,
  onAction,
}: {
  participants: readonly ConversationWorkingParticipant[];
  t: TranslateFn;
  onAction?: (participant?: ConversationWorkingParticipant) => void;
}) {
  return (
    <div className="composer-working">
      <div className="composer-working-status" role="status" aria-live="polite">
        {participants.map((participant) => {
          const action = participant.activity?.action || ConversationWorkingActions.thinking;
          const actionLabel = workingActionLabel(action, t);
          const summary = participant.activity?.summary?.trim() || "";
          const content = (
            <>
              <span className="composer-working-dots" aria-hidden="true">
                <span />
                <span />
                <span />
              </span>
              <strong className="composer-working-name">{participant.name}</strong>
              <span className="composer-working-verb">{actionLabel}</span>
              {summary ? <span className="composer-working-summary">{summary}</span> : null}
            </>
          );
          return onAction ? (
            <button
              key={participant.id || participant.name}
              type="button"
              className="composer-working-item"
              data-working-action={action}
              aria-label={t("conversationWorkingOpenActivity", {
                detail: summary || actionLabel,
                name: participant.name,
              })}
              title={summary || actionLabel}
              onClick={() => onAction(participant)}
            >
              {content}
            </button>
          ) : (
            <div
              key={participant.id || participant.name}
              className="composer-working-item"
              data-working-action={action}
            >
              {content}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function workingActionLabel(action: ConversationWorkingAction, t: TranslateFn): string {
  switch (action) {
    case ConversationWorkingActions.editing:
      return t("conversationWorkingEditing");
    case ConversationWorkingActions.reading:
      return t("conversationWorkingReading");
    case ConversationWorkingActions.replying:
      return t("conversationWorkingReplying");
    case ConversationWorkingActions.running:
      return t("conversationWorkingRunning");
    case ConversationWorkingActions.searching:
      return t("conversationWorkingSearching");
    case ConversationWorkingActions.usingTool:
      return t("conversationWorkingUsingTool");
    case ConversationWorkingActions.waiting:
      return t("conversationWorkingWaiting");
    default:
      return t("conversationWorkingThinking");
  }
}

type ComposerAddMenuProps = {
  busyAction: string;
  busyProvider: string;
  disabled: boolean;
  error: string;
  pending: boolean;
  status: ConnectorStatus;
  gitlabStatus: ConnectorStatus;
  t: TranslateFn;
  onAddFiles: () => void;
  onConnect?: () => VoidOrPromise;
  onDisconnect?: () => VoidOrPromise;
  onDisconnectGitLab?: () => VoidOrPromise;
  onManage?: () => VoidOrPromise;
  onSaveGitLab?: (draft: GitLabConnectorConfigDraft) => VoidOrPromise;
};

function ComposerAddMenu({
  busyAction,
  busyProvider,
  disabled,
  error,
  pending,
  status,
  gitlabStatus,
  t,
  onAddFiles,
  onConnect,
  onDisconnect,
  onDisconnectGitLab,
  onManage,
  onSaveGitLab,
}: ComposerAddMenuProps) {
  const [gitlabFormOpen, setGitLabFormOpen] = useState(false);
  const [gitlabDraft, setGitLabDraft] = useState(() => gitLabConnectorDraftFromStatus(gitlabStatus));
  const accountLabel = status.account?.login || status.account?.name || "";
  const connectorStateLabel =
    status.connected && accountLabel
      ? accountLabel
      : status.connected
        ? t("connectorConnected")
        : t("connectorNotConnected");
  const githubBusy = pending || (busyProvider !== "gitlab" && busyAction === "connect");
  const gitlabBusy = busyProvider === "gitlab" && Boolean(busyAction);

  function handleConnectGitHub() {
    void onConnect?.();
  }

  function handleDisconnectGitHub() {
    void onDisconnect?.();
  }

  function handleManageGitHub() {
    void onManage?.();
  }

  function handleOpenGitLabForm() {
    setGitLabDraft(gitLabConnectorDraftFromStatus(gitlabStatus));
    setGitLabFormOpen(true);
  }

  async function handleSaveGitLab() {
    if (!gitlabDraft.base_url.trim() || (!gitlabStatus.access_token_set && !gitlabDraft.access_token.trim())) return;
    try {
      await onSaveGitLab?.(gitlabDraft);
      setGitLabFormOpen(false);
      setGitLabDraft((current) => ({ ...current, access_token: "" }));
    } catch (_) {
      // The controller owns the localized error shown below the connector list.
    }
  }

  return (
    <PopoverRoot>
      <Tooltip content={t("composerAddContent")}>
        <PopoverTrigger asChild>
          <span>
            <Button
              aria-haspopup="dialog"
              aria-label={t("composerAddContent")}
              className="composer-add-button"
              disabled={disabled}
              iconOnly
              size="lg"
              variant="tertiaryGray"
            >
              <Plus aria-hidden="true" size={24} strokeWidth={1.8} />
            </Button>
          </span>
        </PopoverTrigger>
      </Tooltip>
      <PopoverContent aria-label={t("composerAddContent")} className="composer-add-popover" role="dialog" side="top">
        <section className="composer-add-section" aria-label={t("composerAdd")}>
          <div className="composer-add-section-label">{t("composerAdd")}</div>
          <PopoverClose asChild>
            <button type="button" className="composer-add-menu-item" onClick={onAddFiles}>
              <Paperclip aria-hidden="true" size={19} />
              <span>{t("composerFiles")}</span>
            </button>
          </PopoverClose>
        </section>
        <div className="composer-add-separator" />
        <section className="composer-add-section" aria-label={t("composerConnectors")}>
          <div className="composer-add-section-label">{t("composerConnectors")}</div>
          <div className="connector-provider-row">
            <div className="connector-provider-main">
              <span className="connector-provider-icon" aria-hidden="true">
                {IconImage("github")}
              </span>
              <div className="connector-provider-copy">
                <strong>{t("connectorGitHub")}</strong>
                <span>{connectorStateLabel}</span>
              </div>
            </div>
            {status.connected ? (
              <div className="connector-provider-actions">
                <span className="connector-connected-state">{t("connectorConnected")}</span>
                {status.app_manageable ? (
                  <Button
                    aria-busy={busyAction === "manage" ? true : undefined}
                    className="connector-manage-button"
                    loading={busyAction === "manage"}
                    size="sm"
                    variant="secondaryGray"
                    onClick={handleManageGitHub}
                  >
                    {t("connectorManage")}
                  </Button>
                ) : null}
                <Button
                  aria-busy={busyAction === "disconnect" ? true : undefined}
                  className="connector-disconnect-button connector-disconnect-button-danger"
                  loading={busyAction === "disconnect"}
                  size="sm"
                  variant="outlineDanger"
                  onClick={handleDisconnectGitHub}
                >
                  {t("connectorDisconnect")}
                </Button>
              </div>
            ) : (
              <Button
                aria-busy={githubBusy ? true : undefined}
                className="connector-connect-button"
                loading={githubBusy}
                size="sm"
                variant="tertiaryGray"
                onClick={handleConnectGitHub}
              >
                {t("connectorConnect")}
              </Button>
            )}
          </div>
          <div className="connector-provider-row">
            <div className="connector-provider-main">
              <span className="connector-provider-icon" aria-hidden="true">
                <GitBranch size={16} strokeWidth={1.8} />
              </span>
              <div className="connector-provider-copy">
                <strong>{t("connectorGitLab")}</strong>
                <span>
                  {gitlabStatus.account?.login ||
                    (gitlabStatus.connected ? t("connectorConnected") : t("connectorNotConnected"))}
                </span>
              </div>
            </div>
            {gitlabStatus.connected ? (
              <div className="connector-provider-actions">
                <span className="connector-connected-state">{t("connectorConnected")}</span>
                <Button size="sm" variant="secondaryGray" onClick={handleOpenGitLabForm}>
                  {t("connectorEdit")}
                </Button>
                <Button
                  className="connector-disconnect-button connector-disconnect-button-danger"
                  loading={gitlabBusy && busyAction === "disconnect"}
                  size="sm"
                  variant="outlineDanger"
                  onClick={() => void onDisconnectGitLab?.()}
                >
                  {t("connectorDisconnect")}
                </Button>
              </div>
            ) : (
              <Button
                className="connector-connect-button"
                size="sm"
                variant="tertiaryGray"
                onClick={handleOpenGitLabForm}
              >
                {t("connectorConnect")}
              </Button>
            )}
          </div>
          {gitlabFormOpen ? (
            <div className="connector-gitlab-form">
              <label>
                <span>{t("connectorGitLabBaseURL")}</span>
                <TextInput
                  aria-label={t("connectorGitLabBaseURL")}
                  autoComplete="url"
                  placeholder="https://gitlab.example.com"
                  value={gitlabDraft.base_url}
                  onChange={(event) =>
                    setGitLabDraft((current) => ({ ...current, base_url: event.currentTarget.value }))
                  }
                />
              </label>
              <label>
                <span>{t("connectorGitLabToken")}</span>
                <TextInput
                  aria-label={t("connectorGitLabToken")}
                  autoComplete="off"
                  placeholder={gitlabStatus.access_token_set ? t("connectorGitLabTokenKeep") : "glpat-…"}
                  type="password"
                  value={gitlabDraft.access_token}
                  onChange={(event) =>
                    setGitLabDraft((current) => ({ ...current, access_token: event.currentTarget.value }))
                  }
                />
              </label>
              <div className="connector-gitlab-form-actions">
                <Button size="sm" variant="tertiaryGray" onClick={() => setGitLabFormOpen(false)}>
                  {t("cancel")}
                </Button>
                <Button
                  loading={gitlabBusy && busyAction === "save"}
                  size="sm"
                  disabled={
                    !gitlabDraft.base_url.trim() || (!gitlabStatus.access_token_set && !gitlabDraft.access_token.trim())
                  }
                  onClick={() => void handleSaveGitLab()}
                >
                  {t("connectorSave")}
                </Button>
              </div>
            </div>
          ) : null}
          {pending ? (
            <div className="connector-pending" role="status">
              {t("connectorOAuthPending")}
            </div>
          ) : null}
          {error ? <div className="form-error connector-form-error">{error}</div> : null}
        </section>
      </PopoverContent>
    </PopoverRoot>
  );
}

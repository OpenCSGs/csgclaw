import { memo, useMemo, useRef } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, RefObject } from "react";
import { ArrowUp, Paperclip, Plus } from "lucide-react";
import { CLIProxyAuthControl } from "@/components/business/ProfileControls";
import { Button, PopoverClose, PopoverContent, PopoverRoot, PopoverTrigger } from "@/components/ui";
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
import { emptyGitHubConnectorStatus } from "@/models/connectors";
import type { ConnectorConfigDraft, ConnectorStatus } from "@/models/connectors";
import type { TranslateFn } from "@/models/conversations";
import type { SlashPickerCandidate } from "@/models/slashCommands";
import { MentionPicker } from "./MentionPicker";
import { SlashPicker } from "./SlashPicker";
import { AttachmentDraftStrip } from "./ConversationAttachments";
import { filesFromDataTransfer } from "./attachmentFiles";
import type { ConversationWorkingParticipant, MentionPickerUser, VoidOrPromise } from "./types";

export type ConversationComposerProps = {
  authBusyProvider: string;
  authStatuses: CLIProxyAuthStatusMap;
  connectorBusyAction?: string;
  connectorError?: string;
  connectorPending?: boolean;
  connectorStatus?: ConnectorStatus;
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
  onManageConnector?: () => VoidOrPromise;
  onProviderLogin: (provider: string) => VoidOrPromise;
  onSaveConnectorConfig?: (draft: ConnectorConfigDraft) => VoidOrPromise;
  onSendMessage: () => VoidOrPromise;
  onRemoveAttachment?: (id: string) => void;
  onSyncComposer: () => void;
  onWorkingAction?: () => void;
  slashCandidates: SlashPickerCandidate[];
  slashIndex: number;
  slashPickerLoading: boolean;
  slashPickerOpen: boolean;
  t: TranslateFn;
  workingActionAttention?: boolean;
  workingActionLabel?: string;
  workingParticipants?: ConversationWorkingParticipant[];
};

export const ConversationComposer = memo(function ConversationComposer({
  authBusyProvider,
  authStatuses,
  connectorBusyAction = "",
  connectorError = "",
  connectorPending = false,
  connectorStatus,
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
  workingActionAttention = false,
  workingActionLabel = "",
  workingParticipants = [],
  onApplyMention,
  onApplySlashCandidate,
  onAddAttachments = () => {},
  onComposerCompositionEnd,
  onComposerCompositionStart,
  onComposerKeyDown,
  onConnectConnector,
  onDisconnectConnector,
  onManageConnector,
  onProviderLogin,
  onRemoveAttachment = () => {},
  onSendMessage,
  onSyncComposer,
  onWorkingAction,
}: ConversationComposerProps) {
  const defaultConnectorStatus = useMemo(() => emptyGitHubConnectorStatus(), []);
  const githubStatus = connectorStatus ?? defaultConnectorStatus;
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const sendDisabled = composerDisabled || (!draftText.trim() && attachmentDrafts.length === 0);

  function handleFiles(files: File[]) {
    if (composerDisabled || files.length === 0) {
      return;
    }
    onAddAttachments(files);
  }

  return (
    <footer className="composer">
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
        <ComposerWorkingIndicator
          actionAttention={workingActionAttention}
          actionLabel={workingActionLabel}
          participants={workingParticipants}
          t={t}
          onAction={onWorkingAction}
        />
      ) : null}
      <div
        className="composer-box"
        onDragOver={(event) => {
          if (composerDisabled || filesFromDataTransfer(event.dataTransfer).length === 0) {
            return;
          }
          event.preventDefault();
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
            aria-label={t("inputPlaceholder")}
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
            disabled={composerDisabled}
            error={connectorError}
            pending={connectorPending}
            status={githubStatus}
            t={t}
            onAddFiles={() => fileInputRef.current?.click()}
            onConnect={onConnectConnector}
            onDisconnect={onDisconnectConnector}
            onManage={onManageConnector}
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
          <Button
            variant="primary"
            className="composer-send-button"
            aria-label={t("send")}
            title={t("send")}
            disabled={sendDisabled}
            iconOnly
            size="lg"
            onClick={onSendMessage}
          >
            <ArrowUp aria-hidden="true" size={22} strokeWidth={2.25} />
          </Button>
        </div>
      </div>
      {composerError ? <div className="form-error composer-error">{composerError}</div> : null}
    </footer>
  );
});

function ComposerWorkingIndicator({
  actionAttention,
  actionLabel,
  participants,
  t,
  onAction,
}: {
  actionAttention: boolean;
  actionLabel: string;
  participants: readonly ConversationWorkingParticipant[];
  t: TranslateFn;
  onAction?: () => void;
}) {
  return (
    <div className="composer-working">
      <div className="composer-working-status" role="status" aria-live="polite">
        {participants.map((participant) => (
          <span key={participant.id || participant.name} className="composer-working-item">
            <span className="composer-working-dots" aria-hidden="true">
              <span />
              <span />
              <span />
            </span>
            <span>{t("agentWorking", { name: participant.name })}</span>
          </span>
        ))}
      </div>
      {actionLabel && onAction ? (
        <Button
          className={`composer-working-action${actionAttention ? " needs-attention" : ""}`}
          size="sm"
          variant="tertiaryGray"
          onClick={onAction}
        >
          {actionLabel}
        </Button>
      ) : null}
    </div>
  );
}

type ComposerAddMenuProps = {
  busyAction: string;
  disabled: boolean;
  error: string;
  pending: boolean;
  status: ConnectorStatus;
  t: TranslateFn;
  onAddFiles: () => void;
  onConnect?: () => VoidOrPromise;
  onDisconnect?: () => VoidOrPromise;
  onManage?: () => VoidOrPromise;
};

function ComposerAddMenu({
  busyAction,
  disabled,
  error,
  pending,
  status,
  t,
  onAddFiles,
  onConnect,
  onDisconnect,
  onManage,
}: ComposerAddMenuProps) {
  const accountLabel = status.account?.login || status.account?.name || "";
  const connectorStateLabel =
    status.connected && accountLabel
      ? accountLabel
      : status.connected
        ? t("connectorConnected")
        : t("connectorNotConnected");
  const busy = pending || busyAction === "connect";

  function handleConnectGitHub() {
    void onConnect?.();
  }

  function handleDisconnectGitHub() {
    void onDisconnect?.();
  }

  function handleManageGitHub() {
    void onManage?.();
  }

  return (
    <PopoverRoot>
      <PopoverTrigger asChild>
        <Button
          aria-haspopup="dialog"
          aria-label={t("composerAdd")}
          className="composer-add-button"
          disabled={disabled}
          iconOnly
          size="lg"
          title={t("composerAdd")}
          variant="tertiaryGray"
        >
          <Plus aria-hidden="true" size={24} strokeWidth={1.8} />
        </Button>
      </PopoverTrigger>
      <PopoverContent aria-label={t("composerAdd")} className="composer-add-popover" role="dialog" side="top">
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
                aria-busy={busy ? true : undefined}
                className="connector-connect-button"
                loading={busy}
                size="sm"
                variant="tertiaryGray"
                onClick={handleConnectGitHub}
              >
                {t("connectorConnect")}
              </Button>
            )}
          </div>
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

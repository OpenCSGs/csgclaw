import { memo, useEffect, useMemo, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, RefObject } from "react";
import { Link2 } from "lucide-react";
import { CLIProxyAuthControl } from "@/components/business/ProfileControls";
import { Button } from "@/components/ui";
import { IconImage } from "@/components/ui/Icons";
import type { CLIProxyAuthStatusMap } from "@/hooks/workspace/useCLIProxyAuthStatuses";
import type { AgentProfileLike } from "@/models/agents";
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
  editorRef: RefObject<HTMLDivElement | null>;
  managerProfile?: AgentProfileLike | null;
  managerProvider: string;
  mentionCandidates: MentionPickerUser[];
  mentionIndex: number;
  mentionableUsersByName: Map<string, ComposerMentionUser>;
  onApplyMention: (user: MentionPickerUser) => void;
  onApplySlashCandidate: (name: string) => void;
  onComposerCompositionEnd: () => void;
  onComposerCompositionStart: () => void;
  onComposerKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => void;
  onConnectConnector?: () => VoidOrPromise;
  onDisconnectConnector?: () => VoidOrPromise;
  onManageConnector?: () => VoidOrPromise;
  onProviderLogin: (provider: string) => VoidOrPromise;
  onSaveConnectorConfig?: (draft: ConnectorConfigDraft) => VoidOrPromise;
  onSendMessage: () => VoidOrPromise;
  onSyncComposer: () => void;
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
  connectorError = "",
  connectorPending = false,
  connectorStatus,
  composerDisabled,
  composerDisabledReason = "",
  composerError,
  draftSegments,
  draftText,
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
  onComposerCompositionEnd,
  onComposerCompositionStart,
  onComposerKeyDown,
  onConnectConnector,
  onDisconnectConnector,
  onManageConnector,
  onProviderLogin,
  onSendMessage,
  onSyncComposer,
}: ConversationComposerProps) {
  const defaultConnectorStatus = useMemo(() => emptyGitHubConnectorStatus(), []);
  const githubStatus = connectorStatus ?? defaultConnectorStatus;

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
      {workingParticipants.length > 0 ? <ComposerWorkingIndicator participants={workingParticipants} t={t} /> : null}
      <div className="composer-box">
        <div className="composer-input-wrap">
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
              event.preventDefault();
              const pasted = event.clipboardData?.getData("text/plain") ?? "";
              const segments = normalizeTextMentions([{ type: "text", text: pasted }], mentionableUsersByName);
              if (segments.some((segment) => segment.type === "mention")) {
                insertComposerSegmentsAtSelection(segments);
              } else {
                insertPlainTextAtSelection(pasted);
              }
              onSyncComposer();
            }}
          />
          <Button
            variant="primary"
            className="composer-send-button"
            aria-label={t("send")}
            title={t("send")}
            disabled={composerDisabled || !draftText.trim()}
            onClick={onSendMessage}
          >
            <span className="composer-send-main" aria-hidden="true">
              {IconImage("send")}
            </span>
          </Button>
        </div>
      </div>
      {composerError ? <div className="form-error composer-error">{composerError}</div> : null}
      <ConnectorMenu
        busyAction={connectorBusyAction}
        error={connectorError}
        pending={connectorPending}
        status={githubStatus}
        t={t}
        onConnect={onConnectConnector}
        onDisconnect={onDisconnectConnector}
        onManage={onManageConnector}
      />
    </footer>
  );
});

function ComposerWorkingIndicator({
  participants,
  t,
}: {
  participants: readonly ConversationWorkingParticipant[];
  t: TranslateFn;
}) {
  return (
    <div className="composer-working" role="status" aria-live="polite">
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
  );
}

type ConnectorMenuProps = {
  busyAction: string;
  error: string;
  pending: boolean;
  status: ConnectorStatus;
  t: TranslateFn;
  onConnect?: () => VoidOrPromise;
  onDisconnect?: () => VoidOrPromise;
  onManage?: () => VoidOrPromise;
};

function ConnectorMenu({
  busyAction,
  error,
  pending,
  status,
  t,
  onConnect,
  onDisconnect,
  onManage,
}: ConnectorMenuProps) {
  const menuRef = useRef<HTMLDivElement | null>(null);
  const [open, setOpen] = useState(false);
  const accountLabel = status.account?.login || status.account?.name || "";
  const connectorStateLabel =
    status.connected && accountLabel
      ? accountLabel
      : status.connected
        ? t("connectorConnected")
        : t("connectorNotConnected");
  const title = error || (pending ? t("connectorOAuthPending") : t("connectorManagerTitle"));
  const busy = pending || busyAction === "connect";

  useEffect(() => {
    if (!open) {
      return undefined;
    }
    function handlePointerDown(event: MouseEvent) {
      const menu = menuRef.current;
      if (!menu || !(event.target instanceof Node) || menu.contains(event.target)) {
        return;
      }
      setOpen(false);
    }
    document.addEventListener("mousedown", handlePointerDown);
    return () => document.removeEventListener("mousedown", handlePointerDown);
  }, [open]);

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
    <div className="composer-actions-row">
      <div ref={menuRef} className="composer-connector-menu">
        <Button
          active={open}
          aria-expanded={open}
          aria-haspopup="dialog"
          aria-label={t("connectorManagerTitle")}
          className="composer-tool-button"
          iconOnly
          size="sm"
          title={title}
          variant="tertiaryGray"
          onClick={() => setOpen((current) => !current)}
        >
          <Link2 aria-hidden="true" size={18} />
        </Button>
        {open ? (
          <div className="composer-connector-popover" role="dialog" aria-label={t("connectorManagerTitle")}>
            <div className="connector-popover-header">
              <Link2 aria-hidden="true" size={18} />
              <strong>{t("connectorManagerTitle")}</strong>
            </div>
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
          </div>
        ) : null}
      </div>
      <div className="composer-tip">{t("composerTip")}</div>
    </div>
  );
}

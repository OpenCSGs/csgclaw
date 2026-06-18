import { memo } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, RefObject } from "react";
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
import type { TranslateFn } from "@/models/conversations";
import type { SlashPickerCandidate } from "@/models/slashCommands";
import { MentionPicker } from "./MentionPicker";
import { SlashPicker } from "./SlashPicker";
import type { MentionPickerUser, VoidOrPromise } from "./types";

export type ConversationComposerProps = {
  authBusyProvider: string;
  authStatuses: CLIProxyAuthStatusMap;
  composerDisabled: boolean;
  composerError: string;
  draftSegments: ComposerSegment[];
  draftText: string;
  editorRef: RefObject<HTMLDivElement | null>;
  managerProfile?: AgentProfileLike | null;
  managerProvider: string;
  mentionCandidates: MentionPickerUser[];
  mentionIndex: number;
  mentionableUsersByHandle: Map<string, ComposerMentionUser>;
  onApplyMention: (user: MentionPickerUser) => void;
  onApplySlashCandidate: (name: string) => void;
  onComposerCompositionEnd: () => void;
  onComposerCompositionStart: () => void;
  onComposerKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => void;
  onProviderLogin: (provider: string) => VoidOrPromise;
  onSendMessage: () => VoidOrPromise;
  onSyncComposer: () => void;
  slashCandidates: SlashPickerCandidate[];
  slashIndex: number;
  slashPickerLoading: boolean;
  slashPickerOpen: boolean;
  t: TranslateFn;
};

export const ConversationComposer = memo(function ConversationComposer({
  authBusyProvider,
  authStatuses,
  composerDisabled,
  composerError,
  draftSegments,
  draftText,
  editorRef,
  managerProfile,
  managerProvider,
  mentionCandidates,
  mentionIndex,
  mentionableUsersByHandle,
  slashCandidates,
  slashIndex,
  slashPickerLoading,
  slashPickerOpen,
  t,
  onApplyMention,
  onApplySlashCandidate,
  onComposerCompositionEnd,
  onComposerCompositionStart,
  onComposerKeyDown,
  onProviderLogin,
  onSendMessage,
  onSyncComposer,
}: ConversationComposerProps) {
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
      <div className="composer-box">
        <div className="composer-input-wrap">
          {draftSegments.length === 0 ? (
            <div className="composer-placeholder" aria-hidden="true">
              {composerDisabled ? t("profileIncomplete") : t("inputPlaceholder")}
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
              const segments = normalizeTextMentions([{ type: "text", text: pasted }], mentionableUsersByHandle);
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
      <div className="composer-tip">{t("composerTip")}</div>
    </footer>
  );
});

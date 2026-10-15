import { Check, Edit3 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { AgentAvatarContent } from "@/components/business/AgentAvatar";
import { NotifierControls } from "@/components/business/ProfileControls";
import { Button } from "@/components/ui";
import {
  agentToDraft,
  isAgentIncomplete,
  isAgentRunning,
  notificationBotStatusLabel,
  notifierFormIsComplete,
} from "@/models/agents";
import type { AgentDraft } from "@/models/agents";
import { avatarFallbackText } from "@/shared/avatar";
import type { AgentDetailPaneProps } from "../AgentDetailPane";

export type NotificationBotDetailPaneProps = Pick<
  AgentDetailPaneProps,
  | "busyKey"
  | "error"
  | "item"
  | "notifierWebhookPublicOrigin"
  | "onDelete"
  | "onDraftChange"
  | "onOpenDM"
  | "onSave"
  | "saveError"
  | "saving"
  | "t"
> & {
  draft?: AgentDraft | null;
};

export function NotificationParticipantDetailPane({
  item,
  t,
  busyKey = "",
  error = "",
  saveError = "",
  draft,
  saving = false,
  notifierWebhookPublicOrigin = "",
  onDraftChange,
  onSave,
  onOpenDM,
  onDelete,
}: NotificationBotDetailPaneProps) {
  const [isEditingDescription, setIsEditingDescription] = useState(false);
  const descriptionInputRef = useRef<HTMLTextAreaElement | null>(null);
  const draftBelongsToItem = Boolean(draft) && String(draft?.agent_id ?? "").trim() === String(item?.id ?? "").trim();
  const incomplete = isAgentIncomplete(item, draftBelongsToItem ? draft : undefined);
  const ready = isAgentRunning(item);
  const busyPrefix = `${item.id}:`;
  const updateDraft = (patch: Partial<AgentDraft>) => onDraftChange?.({ ...(draft || agentToDraft(item)), ...patch });

  useEffect(() => {
    if (!draft) {
      setIsEditingDescription(false);
    }
  }, [draft]);

  useEffect(() => {
    if (!isEditingDescription) {
      return;
    }
    descriptionInputRef.current?.focus();
  }, [isEditingDescription]);

  return (
    <section className="entity-pane agent-detail-pane notification-participant-detail-pane">
      <header className="entity-header">
        <div className="entity-avatar">
          <AgentAvatarContent avatar={item.avatar} fallback={avatarFallbackText(item.avatar, item.name, item.id)} />
        </div>
        <div className="entity-heading">
          <div className="entity-title-row">
            <h1>{item.name}</h1>
            <span className={`status-pill ${ready ? "online" : ""}`}>{notificationBotStatusLabel(item, t)}</span>
            <span className={`status-pill profile-state-pill ${incomplete ? "warn" : "ready"}`}>
              {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
            </span>
          </div>
          {draft ? (
            isEditingDescription ? (
              <label className="field entity-description-field">
                <span className="sr-only">{t("agentDescription")}</span>
                <textarea
                  ref={descriptionInputRef}
                  className="compact-textarea"
                  value={draft.description}
                  onBlur={() => setIsEditingDescription(false)}
                  onInput={(event) => updateDraft({ description: event.currentTarget.value })}
                  onKeyDown={(event) => {
                    if (event.key === "Escape") {
                      event.preventDefault();
                      event.currentTarget.blur();
                    }
                  }}
                  placeholder={t("agentDescription")}
                />
              </label>
            ) : (
              <button
                type="button"
                className={`entity-description-display ${draft.description ? "" : "is-empty"}`.trim()}
                aria-label={t("editDescription")}
                onClick={() => setIsEditingDescription(true)}
              >
                <span className="entity-description-display-copy">{draft.description || t("agentDescription")}</span>
                <span className="entity-description-display-icon" aria-hidden="true">
                  <Edit3 size={16} strokeWidth={1.8} />
                </span>
              </button>
            )
          ) : item.description ? (
            <div className="entity-description-text">{item.description}</div>
          ) : null}
        </div>
        <div className="entity-toolbar">
          {draft && !saving && notifierFormIsComplete(draft, item) ? (
            <span className="agent-save-status" role="status">
              <Check aria-hidden="true" size={16} strokeWidth={2.5} />
              {t("agentSaved")}
            </span>
          ) : null}
          <Button
            variant="primary"
            size="md"
            disabled={saving || !draft || !notifierFormIsComplete(draft, item)}
            onClick={onSave}
          >
            {saving ? t("profileLoadingModels") : t("agentUpdateSave")}
          </Button>
          <Button
            variant="secondaryGray"
            size="md"
            disabled={busyKey.startsWith(busyPrefix)}
            onClick={() => onOpenDM(item)}
          >
            {t("openDM")}
          </Button>
          <Button variant="danger" size="md" disabled={busyKey.startsWith(busyPrefix)} onClick={() => onDelete(item)}>
            {t("agentDelete")}
          </Button>
        </div>
      </header>
      {error ? <div className="form-error">{error}</div> : null}
      {saveError ? <div className="form-error">{saveError}</div> : null}
      {draft ? (
        <div className="profile-editor-shell agent-page-editor">
          <NotifierControls
            agentID={item.id || ""}
            draft={draft}
            t={t}
            webhookPublicOrigin={notifierWebhookPublicOrigin}
            onPatch={(patch) => updateDraft(patch)}
          />
        </div>
      ) : null}
    </section>
  );
}

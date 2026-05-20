import { NotifierControls, requiredFieldLabel } from "@/components/business/ProfileControls";
import { AgentIcon } from "@/components/ui/Icons";
import { Button } from "@/components/ui";
import {
  agentToDraft,
  isAgentIncomplete,
  isAgentRunning,
  notificationBotStatusLabel,
  notifierFormIsComplete,
} from "@/models/agents";

export function NotificationBotDetailPane({
  item,
  t,
  busyKey,
  error,
  saveError,
  draft,
  saving,
  notifierWebhookPublicOrigin,
  onDraftChange,
  onSave,
  onOpenDM,
  onDelete,
}) {
  const draftBelongsToItem = Boolean(draft) && String(draft?.agent_id ?? "").trim() === String(item?.id ?? "").trim();
  const incomplete = isAgentIncomplete(item, draftBelongsToItem ? draft : undefined);
  const ready = isAgentRunning(item);
  const busyPrefix = `${item.id}:`;
  const updateDraft = (patch) => onDraftChange?.({ ...(draft || agentToDraft(item)), ...patch });

  return (
    <section className="entity-pane agent-detail-pane notification-bot-detail-pane">
      <header className="entity-header">
        <div className="entity-avatar">
          <AgentIcon />
        </div>
        <div className="entity-heading">
          <div className="entity-title-row">
            <h1>{item.name}</h1>
            <span className={`status-pill ${ready ? "online" : ""}`}>{notificationBotStatusLabel(item, t)}</span>
          </div>
          <p>{item.description || ""}</p>
        </div>
      </header>
      <div className="entity-toolbar">
        <Button
          variant="primary"
          className="preview-action-button preview-action-button-primary"
          disabled={saving || !draft || !notifierFormIsComplete(draft, item)}
          onClick={onSave}
        >
          {saving ? t("profileLoadingModels") : t("agentUpdateSave")}
        </Button>
        <Button
          className="preview-action-button"
          disabled={busyKey.startsWith(busyPrefix)}
          onClick={() => onOpenDM(item)}
        >
          {t("openDM")}
        </Button>
        <Button
          variant="outlineDanger"
          className="preview-action-button preview-action-button-danger"
          disabled={busyKey.startsWith(busyPrefix)}
          onClick={() => onDelete(item)}
        >
          {t("agentDelete")}
        </Button>
      </div>
      {error ? <div className="form-error">{error}</div> : null}
      {saveError ? <div className="form-error">{saveError}</div> : null}
      <div className="entity-badge-row">
        <span className={`agent-badge ${incomplete ? "warn" : ""}`}>
          {incomplete ? t("profileIncompleteBadge") : t("profileCompleteBadge")}
        </span>
      </div>
      {draft ? (
        <div className="profile-editor-shell agent-page-editor">
          <section className="profile-section">
            <div className="profile-section-title">{t("profileBasics")}</div>
            <div className="profile-grid profile-grid-compact">
              <label className="field">
                {requiredFieldLabel(t("agentName"))}
                <input value={draft.name} readOnly disabled required aria-required="true" />
              </label>
              <label className="field span-2">
                <span>{t("agentDescription")}</span>
                <textarea
                  className="compact-textarea"
                  value={draft.description}
                  onInput={(event) => updateDraft({ description: event.currentTarget.value })}
                />
              </label>
            </div>
          </section>
          <NotifierControls
            agentID={item.id}
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

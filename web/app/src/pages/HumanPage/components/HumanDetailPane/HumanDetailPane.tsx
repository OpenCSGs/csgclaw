import { Check, CheckCircle2, Edit3, Link2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { localizeRole } from "@/shared/i18n";
import { feishuHumanParticipant } from "@/models/conversations";
import type { IMUser, LocaleCode, TranslateFn } from "@/models/conversations";

type VoidOrPromise = void | Promise<void>;

export type HumanDetailPaneProps = {
  avatarBusy?: boolean;
  avatarError?: string;
  descriptionBusy?: boolean;
  descriptionError?: string;
  locale?: LocaleCode;
  onAvatarChange?: (avatar: string) => VoidOrPromise;
  onDescriptionSave?: (description: string) => VoidOrPromise;
  t?: TranslateFn;
  user?: IMUser | null;
};

export function HumanDetailPane({
  avatarBusy = false,
  avatarError = "",
  descriptionBusy = false,
  descriptionError = "",
  onAvatarChange = () => {},
  onDescriptionSave = () => {},
  t = (key) => key,
  user = null,
}: HumanDetailPaneProps) {
  const [descriptionDraft, setDescriptionDraft] = useState("");
  const [isEditingDescription, setIsEditingDescription] = useState(false);
  const descriptionInputRef = useRef<HTMLTextAreaElement | null>(null);
  const skipDescriptionAutosaveRef = useRef(false);

  useEffect(() => {
    setDescriptionDraft(String(user?.description || ""));
  }, [user?.description, user?.id]);

  useEffect(() => {
    if (!isEditingDescription) {
      return;
    }
    descriptionInputRef.current?.focus();
  }, [isEditingDescription]);

  if (!user) {
    return (
      <section className="entity-pane human-detail-pane">
        <div className="empty-state shell-empty-state">
          <strong>{t("humanDetailMissing")}</strong>
          <span>{t("humanDetailMissingHint")}</span>
        </div>
      </section>
    );
  }

  const displayName = user.name || user.id;
  const role = localizeRole(user.role || "admin", t);
  const online = user.is_online !== false;
  const currentDescription = String(user.description || "");
  const descriptionChanged = descriptionDraft.trim() !== currentDescription.trim();
  const saveDescriptionIfChanged = (description: string) => {
    if (skipDescriptionAutosaveRef.current) {
      skipDescriptionAutosaveRef.current = false;
      return;
    }
    if (!descriptionBusy && description.trim() !== currentDescription.trim()) {
      void onDescriptionSave(description);
    }
  };
  const cancelDescriptionEdit = () => {
    skipDescriptionAutosaveRef.current = true;
    setDescriptionDraft(currentDescription);
    setIsEditingDescription(false);
  };

  return (
    <section className="entity-pane human-detail-pane">
      <header className="human-overview-card">
        <div className="entity-header human-detail-header">
          <div className="entity-avatar human-detail-avatar agent-header-avatar-picker" aria-busy={avatarBusy}>
            <AgentAvatarPicker
              disabled={avatarBusy}
              value={user.avatar}
              t={t}
              mode="edit"
              onChange={(avatar) => void onAvatarChange(avatar)}
            />
          </div>
          <div className="entity-heading">
            <div className="entity-title-row">
              <h1>{displayName}</h1>
              <span className={`status-pill ${online ? "online" : ""}`}>
                {online ? t("humanStatusOnline") : t("humanStatusOffline")}
              </span>
            </div>
            {isEditingDescription ? (
              <div className="human-description-editor">
                <label className="human-description-field">
                  <span className="sr-only">{t("humanDescriptionLabel")}</span>
                  <textarea
                    ref={descriptionInputRef}
                    value={descriptionDraft}
                    rows={4}
                    disabled={descriptionBusy}
                    onBlur={(event) => {
                      setIsEditingDescription(false);
                      saveDescriptionIfChanged(event.currentTarget.value);
                    }}
                    onChange={(event) => setDescriptionDraft(event.currentTarget.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Escape") {
                        event.preventDefault();
                        cancelDescriptionEdit();
                        event.currentTarget.blur();
                      }
                    }}
                    placeholder={t("humanDescriptionLabel")}
                  />
                </label>
              </div>
            ) : (
              <button
                type="button"
                className={`entity-description-display ${descriptionDraft ? "" : "is-empty"}`.trim()}
                aria-label={t("humanDescriptionLabel")}
                onClick={() => setIsEditingDescription(true)}
              >
                <span className="entity-description-display-copy">
                  {descriptionDraft || t("humanDescriptionLabel")}
                </span>
                <span className="entity-description-display-icon" aria-hidden="true">
                  <Edit3 size={16} strokeWidth={1.8} />
                </span>
              </button>
            )}
            {avatarBusy || avatarError ? (
              <div className="human-avatar-feedback">
                {avatarBusy ? (
                  <span className="human-avatar-status" role="status">
                    {t("humanAvatarSaving")}
                  </span>
                ) : null}
                {avatarError ? (
                  <span className="human-avatar-error" role="alert">
                    {avatarError}
                  </span>
                ) : null}
              </div>
            ) : null}
            {descriptionError ? (
              <div className="human-avatar-feedback">
                <span className="human-avatar-error" role="alert">
                  {descriptionError}
                </span>
              </div>
            ) : null}
          </div>
          <div className="entity-toolbar human-detail-toolbar">
            {descriptionBusy ? (
              <span className="human-save-status" role="status">
                {t("agentSavingChanges")}
              </span>
            ) : !descriptionChanged ? (
              <span className="human-save-status" role="status">
                <Check aria-hidden="true" size={16} strokeWidth={2.5} />
                {t("agentSaved")}
              </span>
            ) : null}
          </div>
        </div>
      </header>

      <section className="human-info-panel">
        <HumanChannelsSection t={t} user={user} />
        <section className="human-info-section human-identity-section">
          <div className="section-header-inline human-info-section-header">
            <div className="section-label">{t("humanIdentitySection")}</div>
          </div>
          <div className="human-identity-fields">
            <HumanField label={t("roleLabel")} value={role} />
            <HumanField label={t("userIDLabel")} value={user.user_id || user.id} />
          </div>
        </section>
      </section>
    </section>
  );
}

function HumanChannelsSection({ t, user }: { t: TranslateFn; user: IMUser }) {
  const feishuParticipant = feishuHumanParticipant(user);
  const connected = Boolean(feishuParticipant);
  const statusLabel = connected ? t("feishuConnected") : t("feishuDisconnected");
  const statusIcon = connected ? (
    <CheckCircle2 aria-hidden="true" size={16} strokeWidth={2.2} />
  ) : (
    <Link2 aria-hidden="true" size={16} strokeWidth={2.2} />
  );

  return (
    <section className="human-info-section human-channels-section" aria-labelledby="human-channels-title">
      <div className="section-header-inline human-info-section-header">
        <h2 id="human-channels-title" className="section-label">
          {t("humanChannelsSection")}
        </h2>
      </div>
      <div className="human-channel-row">
        <span className="human-channel-icon" aria-hidden="true">
          <img src="icons/feishu.png" alt="" />
        </span>
        <span className="human-channel-main">
          <span className="human-channel-name">{t("feishuChannelName")}</span>
          <span className={`human-channel-status ${connected ? "connected" : ""}`.trim()}>
            {statusIcon}
            {statusLabel}
          </span>
        </span>
      </div>
    </section>
  );
}

function HumanField({ label, value }: { label: string; value: string }) {
  return (
    <div className="entity-field human-field">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

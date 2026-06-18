import { CheckCircle2, Link2, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { Button } from "@/components/ui";
import { localizeRole } from "@/shared/i18n";
import { feishuHumanParticipant, humanParticipantDisplayName } from "@/models/conversations";
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

  useEffect(() => {
    setDescriptionDraft(String(user?.description || ""));
  }, [user?.description, user?.id]);

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

  const displayName = user.name || user.handle || user.id;
  const handle = user.handle ? `@${user.handle}` : "-";
  const role = localizeRole(user.role || "admin", t);
  const online = user.is_online !== false;
  const currentDescription = String(user.description || "");
  const descriptionChanged = descriptionDraft.trim() !== currentDescription.trim();

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
            <p>{t("humanDetailSubtitle")}</p>
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
          </div>
        </div>
      </header>

      <section className="human-info-panel">
        <section className="human-info-section human-identity-section">
          <div className="section-header-inline human-info-section-header">
            <div className="section-label">{t("humanIdentitySection")}</div>
          </div>
          <div className="human-identity-fields">
            <HumanField label={t("roleLabel")} value={role} />
            <HumanField label={t("handleLabel")} value={handle} />
            <HumanField label={t("userIDLabel")} value={user.user_id || user.id} />
          </div>
        </section>
        <section className="human-info-section human-description-section">
          <div className="section-header-inline human-info-section-header">
            <h2 className="section-label">{t("humanDescriptionLabel")}</h2>
          </div>
          <label className="human-description-field">
            <span>{t("humanDescriptionLabel")}</span>
            <textarea
              value={descriptionDraft}
              rows={4}
              disabled={descriptionBusy}
              onChange={(event) => setDescriptionDraft(event.currentTarget.value)}
            />
          </label>
          <div className="human-description-actions">
            {descriptionError ? (
              <span className="human-avatar-error" role="alert">
                {descriptionError}
              </span>
            ) : null}
            <Button
              variant="primary"
              size="sm"
              type="button"
              loading={descriptionBusy}
              loadingLabel={t("humanDescriptionSave")}
              disabled={!descriptionChanged || descriptionBusy}
              onClick={() => void onDescriptionSave(descriptionDraft)}
            >
              <Save aria-hidden="true" size={15} strokeWidth={2} />
              {t("humanDescriptionSave")}
            </Button>
          </div>
        </section>
        <HumanChannelsSection t={t} user={user} />
      </section>
    </section>
  );
}

function HumanChannelsSection({ t, user }: { t: TranslateFn; user: IMUser }) {
  const feishuParticipant = feishuHumanParticipant(user);
  const connected = Boolean(feishuParticipant);
  const boundUser = humanParticipantDisplayName(feishuParticipant);
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
          {boundUser ? (
            <span className="human-channel-detail">
              {t("humanFeishuBoundUser")} <strong>{boundUser}</strong>
            </span>
          ) : null}
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

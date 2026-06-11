import { AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { localizeRole } from "@/shared/i18n";
import type { IMUser, LocaleCode, TranslateFn } from "@/models/conversations";

type VoidOrPromise = void | Promise<void>;

export type HumanDetailPaneProps = {
  avatarBusy?: boolean;
  avatarError?: string;
  locale?: LocaleCode;
  onAvatarChange?: (avatar: string) => VoidOrPromise;
  t?: TranslateFn;
  user?: IMUser | null;
};

export function HumanDetailPane({
  avatarBusy = false,
  avatarError = "",
  onAvatarChange = () => {},
  t = (key) => key,
  user = null,
}: HumanDetailPaneProps) {
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
      </section>
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

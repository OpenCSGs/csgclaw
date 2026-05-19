// @ts-nocheck
import { Button } from "@/components/ui";
import { formatRuntimeKindLabel, normalizeRuntimeKind, runtimeImageForKind } from "@/models/agents";

export function ManagerRebuildModal({
  t,
  runtimeOptions,
  runtimeKind,
  image,
  bootstrapConfig,
  managerAgent,
  busy,
  error,
  onRuntimeKindChange,
  onImageChange,
  onClose,
  onConfirm,
}) {
  const selectedRuntimeKind = normalizeRuntimeKind(runtimeKind) || runtimeOptions[0]?.value || "picoclaw_sandbox";
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-card profile-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("managerRebuildTitle")}</div>
            <div className="modal-subtitle">{t("managerRebuildSubtitle")}</div>
          </div>
          <Button className="modal-close" onClick={onClose}>
            {t("close")}
          </Button>
        </div>
        <div className="profile-editor-shell">
          <section className="profile-section">
            <div className="profile-grid profile-grid-compact manager-rebuild-grid">
              <label className="field manager-rebuild-runtime-field">
                <span>{t("profileRuntimeKind")}</span>
                <select
                  value={selectedRuntimeKind}
                  onChange={(event) => {
                    const nextRuntimeKind = normalizeRuntimeKind(event.target.value);
                    onRuntimeKindChange(nextRuntimeKind);
                    onImageChange(runtimeImageForKind(nextRuntimeKind, bootstrapConfig, managerAgent?.image || ""));
                  }}
                >
                  {runtimeOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {formatRuntimeKindLabel(option.value, t)}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field manager-rebuild-image-field">
                <span>{t("agentImage")}</span>
                <input
                  value={image}
                  onInput={(event) => onImageChange(event.target.value)}
                  placeholder={t("agentImagePlaceholder")}
                />
              </label>
            </div>
          </section>
          {error ? <div className="form-error">{error}</div> : null}
          <div className="modal-actions">
            <Button className="secondary-button" disabled={busy} onClick={onClose}>
              {t("close")}
            </Button>
            <Button variant="primary" className="send-button" disabled={busy} onClick={onConfirm}>
              {busy ? t("profileLoadingModels") : t("managerRebuildAction")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

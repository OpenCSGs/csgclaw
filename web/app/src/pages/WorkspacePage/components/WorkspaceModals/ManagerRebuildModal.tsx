import { AgentCreateProgress } from "@/components/business/ProfileControls";
import { Button, Select } from "@/components/ui";
import { defaultManagerRebuildImageForRuntime, formatRuntimeKindLabel, normalizeRuntimeKind } from "@/models/agents";
import { ModalCloseButton } from "./ModalCloseButton";

export function ManagerRebuildModal({
  t,
  runtimeOptions,
  runtimeKind,
  image,
  imageOptions = [],
  templateVariants = [],
  bootstrapConfig,
  managerAgent,
  busy,
  error,
  progress = null,
  onRuntimeKindChange,
  onImageChange,
  onClose,
  onConfirm,
}) {
  const selectedRuntimeKind = normalizeRuntimeKind(runtimeKind) || runtimeOptions[0]?.value || "picoclaw_sandbox";
  return (
    <div className="modal-backdrop">
      <div className="modal-card profile-modal manager-rebuild-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("managerRebuildTitle")}</div>
            <div className="modal-subtitle">{t("managerRebuildSubtitle")}</div>
          </div>
          <ModalCloseButton label={t("close")} onClose={onClose} />
        </div>
        <div className="profile-editor-shell">
          <section className="profile-section">
            <div className="profile-grid profile-grid-compact manager-rebuild-grid">
              <label className="field manager-rebuild-runtime-field">
                <span>{t("profileRuntimeKind")}</span>
                <Select
                  value={selectedRuntimeKind}
                  onValueChange={(value) => {
                    const nextRuntimeKind = normalizeRuntimeKind(value);
                    onRuntimeKindChange(nextRuntimeKind);
                    onImageChange(
                      defaultManagerRebuildImageForRuntime(
                        templateVariants,
                        nextRuntimeKind,
                        bootstrapConfig,
                        managerAgent?.image || "",
                      ),
                    );
                  }}
                  triggerProps={{ "aria-label": t("profileRuntimeKind") }}
                  options={runtimeOptions.map((option) => ({
                    value: option.value,
                    label: formatRuntimeKindLabel(option.value, t),
                  }))}
                />
              </label>
              <label className="field manager-rebuild-image-field">
                <span>{t("agentImage")}</span>
                <input
                  list="manager-rebuild-image-options"
                  value={image}
                  onInput={(event) => onImageChange(event.currentTarget.value)}
                  placeholder={t("agentImagePlaceholder")}
                />
                <datalist id="manager-rebuild-image-options">
                  {imageOptions.map((option) => (
                    <option key={option} value={option} />
                  ))}
                </datalist>
              </label>
            </div>
          </section>
          {error ? <div className="form-error">{error}</div> : null}
          <AgentCreateProgress progress={progress} t={t} />
          <div className="modal-actions">
            <Button variant="secondaryGray" size="md" disabled={busy} onClick={onClose}>
              {t("close")}
            </Button>
            <Button
              className="manager-rebuild-submit"
              variant="primary"
              size="md"
              disabled={busy}
              loading={busy}
              loadingLabel={t("managerRebuildBusy")}
              onClick={onConfirm}
            >
              {t("managerRebuildAction")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

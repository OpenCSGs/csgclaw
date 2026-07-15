import { AgentCreateProgress, type AgentCreateProgressProps } from "@/components/business/ProfileControls";
import { Button, Tooltip } from "@/components/ui";
import { formatRuntimeKindLabel, normalizeRuntimeKind } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { ModalCloseButton } from "./ModalCloseButton";

type RuntimeOption = {
  value: string;
};

type ImageReferenceParts = {
  context: string;
  name: string;
  suffix: string;
};

export type ManagerRebuildModalProps = {
  busy?: boolean;
  error?: string;
  image?: string;
  runtimeWarning?: string;
  onClose: () => void;
  onConfirm: () => void | Promise<void>;
  onRuntimeKindChange: (runtimeKind: string) => void;
  progress?: AgentCreateProgressProps["progress"];
  runtimeKind?: string;
  runtimeOptions: RuntimeOption[];
  t: TranslateFn;
};

export function ManagerRebuildModal({
  t,
  runtimeOptions,
  runtimeKind,
  image,
  runtimeWarning = "",
  busy = false,
  error = "",
  progress = null,
  onClose,
  onConfirm,
}: ManagerRebuildModalProps) {
  const selectedRuntimeKind = normalizeRuntimeKind(runtimeKind) || runtimeOptions[0]?.value || "codex";
  const selectedImage = String(image ?? "").trim();
  const runtimeLabel = formatRuntimeKindLabel(selectedRuntimeKind, t);
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
              <div className="field manager-rebuild-runtime-field">
                <span>{t("profileRuntimeKind")}</span>
                <div className="manager-rebuild-image-select manager-rebuild-image-readonly">{runtimeLabel}</div>
              </div>
              <label className="field manager-rebuild-image-field">
                <span>{t("agentImage")}</span>
                <Tooltip content={selectedImage}>
                  <div className="manager-rebuild-image-select manager-rebuild-image-readonly">
                    {selectedImage ? (
                      <ImageReferenceLabel image={selectedImage} />
                    ) : (
                      <span className="manager-rebuild-image-placeholder">{t("agentImagePlaceholder")}</span>
                    )}
                  </div>
                </Tooltip>
              </label>
            </div>
          </section>
          {runtimeWarning ? <div className="form-error">{runtimeWarning}</div> : null}
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
              disabled={busy || Boolean(runtimeWarning)}
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

function ImageReferenceLabel({ image }: { image: string }) {
  const { context, name, suffix } = imageReferenceParts(image);
  return (
    <Tooltip content={image}>
      <span className="manager-rebuild-image-option">
        <span className="sr-only">{image}</span>
        <span className="manager-rebuild-image-primary" aria-hidden="true">
          <span className="manager-rebuild-image-name">{name}</span>
          {suffix ? <span className="manager-rebuild-image-tag">{suffix}</span> : null}
        </span>
        {context ? (
          <span className="manager-rebuild-image-context" aria-hidden="true">
            {context}
          </span>
        ) : null}
      </span>
    </Tooltip>
  );
}

function imageReferenceParts(image: string): ImageReferenceParts {
  const value = String(image ?? "").trim();
  if (!value) {
    return { context: "", name: "", suffix: "" };
  }
  const digestIndex = value.indexOf("@");
  if (digestIndex > 0) {
    return splitImagePath(value.slice(0, digestIndex), value.slice(digestIndex));
  }
  const lastSlash = value.lastIndexOf("/");
  const lastColon = value.lastIndexOf(":");
  if (lastColon > lastSlash) {
    return splitImagePath(value.slice(0, lastColon), value.slice(lastColon));
  }
  return splitImagePath(value, "");
}

function splitImagePath(path: string, suffix: string): ImageReferenceParts {
  const lastSlash = path.lastIndexOf("/");
  if (lastSlash < 0) {
    return { context: "", name: path, suffix };
  }
  return {
    context: path.slice(0, lastSlash),
    name: path.slice(lastSlash + 1),
    suffix,
  };
}

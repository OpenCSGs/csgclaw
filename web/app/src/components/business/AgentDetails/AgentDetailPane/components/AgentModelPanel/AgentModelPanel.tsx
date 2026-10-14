import { AlertCircle, RefreshCw } from "lucide-react";
import { errorMessage } from "@/api/client";
import { AgentModelFields, ModelPicker } from "@/components/business/AgentModelFields";
import { ModelOptionLabel, ReasoningControls } from "@/components/business/ProfileControls";
import { Button, Select } from "@/components/ui";
import type { AgentDraft } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { modelProviderAvatarPath, type modelProviderSelectOptionsFromCatalog } from "@/models/modelProviders";
import { localizeAPIError } from "@/shared/i18n";

type ModelProviderSelectOption = ReturnType<typeof modelProviderSelectOptionsFromCatalog>[number];
type UpdateAgentDraft = (patch: Partial<AgentDraft>) => void;

type AgentModelPanelProps = {
  draft: AgentDraft;
  modelBusy: boolean;
  modelError: unknown;
  onRetryModels?: () => void | Promise<unknown>;
  onRequireOpenCSGAuth: () => boolean;
  providerOptions: readonly ModelProviderSelectOption[];
  selectedModelValue: string;
  selectedProviderID: string;
  selectedProviderModels: readonly string[];
  t: TranslateFn;
  updateDraft: UpdateAgentDraft;
};

export function AgentModelPanel({
  draft,
  modelBusy,
  modelError,
  onRetryModels,
  onRequireOpenCSGAuth,
  providerOptions,
  selectedModelValue,
  selectedProviderID,
  selectedProviderModels,
  t,
  updateDraft,
}: AgentModelPanelProps) {
  const imageProviderID = draft.image_generation?.provider_id || "";
  const imageModelID = draft.image_generation?.model_id || "";
  const imageProviders = providerOptions.filter(
    (provider) => (provider.imageModels?.length ?? 0) > 0 || provider.id === imageProviderID,
  );
  const selectedImageProvider = imageProviders.find((provider) => provider.id === imageProviderID);
  const imageModels = selectedImageProvider?.imageModels ?? [];
  const imageProviderLabel = imageProviderID ? (
    <ModelOptionLabel
      avatar={selectedImageProvider?.avatar || modelProviderAvatarPath(imageProviderID)}
      model={selectedImageProvider?.displayName || imageProviderID}
    />
  ) : undefined;
  const videoProviderID = draft.video_generation?.provider_id || "";
  const videoModelID = draft.video_generation?.model_id || "";
  const videoProviders = providerOptions.filter(
    (provider) => (provider.videoModels?.length ?? 0) > 0 || provider.id === videoProviderID,
  );
  const selectedVideoProvider = videoProviders.find((provider) => provider.id === videoProviderID);
  const videoModels = selectedVideoProvider?.videoModels ?? [];
  const videoProviderLabel = videoProviderID ? (
    <ModelOptionLabel
      avatar={selectedVideoProvider?.avatar || modelProviderAvatarPath(videoProviderID)}
      model={selectedVideoProvider?.displayName || videoProviderID}
    />
  ) : undefined;

  return (
    <section id="agent-profile-model" className="profile-section agent-profile-scroll-target">
      <div className="profile-section-heading">
        <div className="profile-section-title">{t("profileModelSection")}</div>
        <p className="profile-section-description">{t("profileModelSectionDescription")}</p>
      </div>
      <div className="agent-section-form">
        <div className="agent-page-form-content agent-model-form-content">
          <div className="profile-runtime-grid agent-model-config-grid">
            <AgentModelFields
              providerOptions={providerOptions}
              providerID={selectedProviderID}
              modelID={selectedModelValue}
              models={selectedProviderModels}
              busy={modelBusy}
              onPatch={updateDraft}
              onRequireOpenCSGAuth={onRequireOpenCSGAuth}
              t={t}
            />
            <ReasoningControls
              value={draft.reasoning_effort}
              onChange={(value) => updateDraft({ reasoning_effort: value })}
              t={t}
            />
            <div className="field agent-fast-mode-field">
              <span>{t("profileFastMode")}</span>
              <label className="selection-item compact-toggle-row agent-fast-mode-toggle">
                <input
                  type="checkbox"
                  checked={draft.enable_fast_mode}
                  aria-label={t("profileFastMode")}
                  onChange={() => updateDraft({ enable_fast_mode: !draft.enable_fast_mode })}
                />
                <small className="agent-fast-mode-help">{t("profileFastModeHelp")}</small>
              </label>
            </div>
            <div className="agent-media-model-group agent-image-model-group">
              <label className="field">
                <span>{t("profileImageModelProvider")}</span>
                <Select
                  value={imageProviderID}
                  selectedLabel={imageProviderLabel}
                  onValueChange={(value) => {
                    const provider = imageProviders.find((option) => option.id === value);
                    updateDraft({
                      image_generation: provider?.imageModels?.length
                        ? { provider_id: provider.id, model_id: provider.imageModels[0] }
                        : null,
                    });
                  }}
                  triggerProps={{ "aria-label": t("profileImageModelProvider") }}
                  options={[
                    { value: "", label: t("profileImageModelNone") },
                    ...imageProviders.map((provider) => ({
                      value: provider.id,
                      label: <ModelOptionLabel avatar={provider.avatar} model={provider.displayName} />,
                      textValue: provider.displayName,
                    })),
                    ...(imageProviderID && !selectedImageProvider
                      ? [{ value: imageProviderID, label: imageProviderLabel, textValue: imageProviderID }]
                      : []),
                  ]}
                />
              </label>
              <label className="field">
                <span>{t("profileImageModel")}</span>
                <ModelPicker
                  value={imageModelID}
                  models={imageModels}
                  disabled={!imageProviderID || !imageModels.length}
                  label={t("profileImageModel")}
                  unavailableLabel={t("profileImageModelUnavailable")}
                  placeholder={imageProviderID ? t("profileSelectModel") : t("profileProviderSelectFirst")}
                  onChange={(value) =>
                    updateDraft({ image_generation: value ? { provider_id: imageProviderID, model_id: value } : null })
                  }
                  t={t}
                />
              </label>
              <div className="agent-media-model-help">
                <small>{t("profileImageModelHelp")}</small>
                {!imageProviders.some((provider) => provider.imageModels?.length) ? (
                  <small>{t("profileImageModelEmpty")}</small>
                ) : null}
              </div>
            </div>
            <div className="agent-media-model-group agent-video-model-group">
              <label className="field">
                <span>{t("profileVideoModelProvider")}</span>
                <Select
                  value={videoProviderID}
                  selectedLabel={videoProviderLabel}
                  onValueChange={(value) => {
                    const provider = videoProviders.find((option) => option.id === value);
                    updateDraft({
                      video_generation: provider?.videoModels?.length
                        ? { provider_id: provider.id, model_id: provider.videoModels[0] }
                        : null,
                    });
                  }}
                  triggerProps={{ "aria-label": t("profileVideoModelProvider") }}
                  options={[
                    { value: "", label: t("profileVideoModelNone") },
                    ...videoProviders.map((provider) => ({
                      value: provider.id,
                      label: <ModelOptionLabel avatar={provider.avatar} model={provider.displayName} />,
                      textValue: provider.displayName,
                    })),
                    ...(videoProviderID && !selectedVideoProvider
                      ? [{ value: videoProviderID, label: videoProviderLabel, textValue: videoProviderID }]
                      : []),
                  ]}
                />
              </label>
              <label className="field">
                <span>{t("profileVideoModel")}</span>
                <ModelPicker
                  value={videoModelID}
                  models={videoModels}
                  disabled={!videoProviderID || !videoModels.length}
                  label={t("profileVideoModel")}
                  placeholder={videoProviderID ? t("profileSelectModel") : t("profileProviderSelectFirst")}
                  onChange={(value) =>
                    updateDraft({ video_generation: value ? { provider_id: videoProviderID, model_id: value } : null })
                  }
                  t={t}
                />
              </label>
              <div className="agent-media-model-help">
                <small>{t("profileVideoModelHelp")}</small>
                {!videoProviders.some((provider) => provider.videoModels?.length) ? (
                  <small>{t("profileVideoModelEmpty")}</small>
                ) : null}
              </div>
            </div>
            {modelError ? (
              <div className="agent-model-load-error" role="alert">
                <AlertCircle className="agent-model-load-error-icon" aria-hidden="true" size={20} strokeWidth={2} />
                <div className="agent-model-load-error-content">
                  <strong>{t("modelLoadFailed")}</strong>
                  <p>{t("profileModelLoadErrorHelp")}</p>
                  {selectedModelValue ? <small>{t("profileModelCurrentSelectionRetained")}</small> : null}
                  <details className="agent-model-load-error-details">
                    <summary>{t("profileModelErrorDetails")}</summary>
                    <div className="agent-model-load-error-technical">
                      <pre>{localizeAPIError(modelError, t, errorMessage(modelError, t("modelLoadFailed")))}</pre>
                    </div>
                  </details>
                </div>
                {onRetryModels ? (
                  <Button
                    className="agent-model-load-error-retry"
                    loading={modelBusy}
                    loadingLabel={t("profileLoadingModels")}
                    size="sm"
                    variant="secondaryGray"
                    onClick={() => void onRetryModels()}
                  >
                    <RefreshCw aria-hidden="true" size={15} strokeWidth={2} />
                    {t("retry")}
                  </Button>
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
      </div>
    </section>
  );
}

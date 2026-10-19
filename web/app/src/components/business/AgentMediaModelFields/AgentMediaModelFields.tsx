import { ModelPicker } from "@/components/business/AgentModelFields";
import { ModelOptionLabel } from "@/components/business/ProfileControls";
import { Select } from "@/components/ui";
import type { AgentDraft } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { modelProviderAvatarPath, type ModelProviderSelectOption } from "@/models/modelProviders";
import styles from "./AgentMediaModelFields.module.css";

export function AgentMediaModelFields({
  draft,
  providerOptions,
  updateDraft,
  busy = false,
  t,
}: {
  draft: AgentDraft;
  providerOptions: readonly ModelProviderSelectOption[];
  updateDraft: (patch: Partial<AgentDraft>) => void;
  busy?: boolean;
  t: TranslateFn;
}) {
  const imageProviderID = draft.image_generation?.provider_id || "";
  const imageModelID = draft.image_generation?.model_id || "";
  const imageProviders = providerOptions.filter(
    (provider) => !provider.disabled && (provider.imageModels?.length ?? 0) > 0,
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
    (provider) => !provider.disabled && (provider.videoModels?.length ?? 0) > 0,
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
    <>
      <fieldset>
        <legend>{t("profileImageModelGroup")}</legend>
        <div className={styles.group}>
          <label className="field">
            <span>{t("profileImageModelProvider")}</span>
            <Select
              disabled={busy}
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
                { value: "", label: t("modelDefaultNone") },
                ...imageProviders.map((provider) => ({
                  value: provider.id,
                  disabled: provider.disabled,
                  label: <ModelOptionLabel avatar={provider.avatar} model={provider.displayName} />,
                  textValue: provider.displayName,
                })),
              ]}
            />
          </label>
          <label className="field">
            <span>{t("profileImageModel")}</span>
            <ModelPicker
              value={imageModelID}
              models={imageModels}
              disabled={busy || selectedImageProvider?.disabled || !imageProviderID || !imageModels.length}
              label={t("profileImageModel")}
              unavailableLabel={t("profileImageModelUnavailable")}
              placeholder={t(imageModels.length ? "profileSelectModel" : "modelDefaultNone")}
              onChange={(value) =>
                updateDraft({ image_generation: value ? { provider_id: imageProviderID, model_id: value } : null })
              }
              t={t}
            />
          </label>
          <div className={styles.help}>
            <small>{t("profileImageModelHelp")}</small>
            {!imageProviders.some((provider) => provider.imageModels?.length) ? (
              <small>{t("profileImageModelEmpty")}</small>
            ) : null}
          </div>
        </div>
      </fieldset>
      <fieldset>
        <legend>{t("profileVideoModelGroup")}</legend>
        <div className={styles.group}>
          <label className="field">
            <span>{t("profileVideoModelProvider")}</span>
            <Select
              disabled={busy}
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
                { value: "", label: t("modelDefaultNone") },
                ...videoProviders.map((provider) => ({
                  value: provider.id,
                  disabled: provider.disabled,
                  label: <ModelOptionLabel avatar={provider.avatar} model={provider.displayName} />,
                  textValue: provider.displayName,
                })),
              ]}
            />
          </label>
          <label className="field">
            <span>{t("profileVideoModel")}</span>
            <ModelPicker
              value={videoModelID}
              models={videoModels}
              disabled={busy || selectedVideoProvider?.disabled || !videoProviderID || !videoModels.length}
              label={t("profileVideoModel")}
              placeholder={t(videoModels.length ? "profileSelectModel" : "modelDefaultNone")}
              onChange={(value) =>
                updateDraft({ video_generation: value ? { provider_id: videoProviderID, model_id: value } : null })
              }
              t={t}
            />
          </label>
          <div className={styles.help}>
            <small>{t("profileVideoModelHelp")}</small>
            {!videoProviders.some((provider) => provider.videoModels?.length) ? (
              <small>{t("profileVideoModelEmpty")}</small>
            ) : null}
          </div>
        </div>
      </fieldset>
    </>
  );
}

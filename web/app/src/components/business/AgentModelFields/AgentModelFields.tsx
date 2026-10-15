import { Field, Select } from "@/components/ui";
import { ModelOptionLabel } from "@/components/business/ProfileControls";
import {
  MODEL_PROVIDER_IDS,
  defaultModelIDForProvider,
  providerNameForProviderID,
  type modelProviderSelectOptionsFromCatalog,
} from "@/models/modelProviders";
import type { AgentDraft } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { ConnectedModelPicker } from "@/components/business/ConnectedModelPicker";

export type AgentModelFieldsProps = {
  providerOptions: Readonly<ReturnType<typeof modelProviderSelectOptionsFromCatalog>>;
  providerID: string;
  modelID: string;
  models: readonly string[];
  busy?: boolean;
  onPatch: (patch: Partial<AgentDraft>) => void;
  onRequireOpenCSGAuth: () => boolean;
  t: TranslateFn;
};

export function AgentModelFields({
  providerOptions,
  providerID,
  modelID,
  models,
  busy = false,
  onPatch,
  onRequireOpenCSGAuth,
  t,
}: AgentModelFieldsProps) {
  const selectedProvider = providerOptions.find((option) => option.id === providerID);
  return (
    <>
      <Field required label={t("profileModelProvider")}>
        <Select
          value={providerID}
          required
          placeholder={busy ? t("profileLoadingModels") : t("profileProviderSelect")}
          selectedLabel={
            selectedProvider ? (
              <ModelOptionLabel avatar={selectedProvider.avatar} model={selectedProvider.displayName} />
            ) : undefined
          }
          triggerProps={{ "aria-label": t("profileModelProvider"), "aria-required": true }}
          onValueChange={(value) => {
            const provider = providerOptions.find((option) => option.id === value);
            if (!provider) {
              onPatch({ model_id: "", model_provider_id: "" });
              return;
            }
            if (provider.disabled || (provider.id === MODEL_PROVIDER_IDS.OpenCSG && !onRequireOpenCSGAuth())) return;
            onPatch({
              provider: providerNameForProviderID(provider.id),
              model_provider_id: provider.id,
              model_id: defaultModelIDForProvider(provider.id, provider.models),
            });
          }}
          options={providerOptions.map((option) => ({
            value: option.value,
            disabled: option.disabled,
            textValue: option.displayName,
            label: (
              <ModelOptionLabel
                avatar={option.avatar}
                model={option.displayName}
                provider={
                  option.models.length
                    ? t("modelProviderModelCount", { count: option.models.length })
                    : t("modelProviderNoModels")
                }
              />
            ),
          }))}
        />
      </Field>
      <Field required label={t("profileModel")}>
        <ConnectedModelPicker
          selectionLabel="model"
          providers={providerOptions.map((option) =>
            option.id === providerID ? { ...option, models: [...models] } : option,
          )}
          providerID={providerID}
          modelID={modelID}
          disabled={busy || providerOptions.every((option) => option.disabled || !option.models.length)}
          onChoose={(id, model) => {
            if (id === MODEL_PROVIDER_IDS.OpenCSG && !onRequireOpenCSGAuth()) return;
            onPatch({ provider: providerNameForProviderID(id), model_provider_id: id, model_id: model });
          }}
          t={t}
        />
      </Field>
    </>
  );
}

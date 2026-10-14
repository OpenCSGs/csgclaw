import { Combobox } from "@/components/ui/Combobox";
import { ModelOptionLabel } from "@/components/business/ProfileControls";
import type { TranslateFn } from "@/models/conversations";
import { isOpenCSGAutoModel } from "@/models/modelProviders";

export type ModelPickerProps = {
  value: string;
  providerID?: string;
  models: readonly string[];
  disabled?: boolean;
  required?: boolean;
  placeholder: string;
  label?: string;
  unavailableLabel?: string;
  onChange: (value: string) => void;
  t: TranslateFn;
};

export function ModelPicker({
  value,
  providerID = "",
  models,
  disabled,
  required,
  placeholder,
  label,
  unavailableLabel,
  onChange,
  t,
}: ModelPickerProps) {
  const uniqueModels = [...new Set(models)];
  const isAuto = (model: string) => isOpenCSGAutoModel(providerID, model);
  const modelLabel = (model: string, showBadge = true) => (
    <ModelOptionLabel
      model={isAuto(model) ? t("modelProviderAutoName") : model}
      badge={isAuto(model) && showBadge ? t("modelProviderAutoBadge") : undefined}
      title={isAuto(model) ? t("modelProviderAutoRouteHint") : undefined}
      showAvatar={false}
    />
  );
  const modelSearchText = (model: string) =>
    isAuto(model) ? `${model} ${t("modelProviderAutoName")} ${t("modelProviderAutoBadge")}` : model;
  return (
    <Combobox
      value={value}
      placeholder={placeholder}
      selectedLabel={value ? modelLabel(value, false) : undefined}
      disabled={disabled}
      required={required}
      onValueChange={onChange}
      triggerProps={{ "aria-label": label ?? t("profileModel"), "aria-required": required }}
      searchPlaceholder={t("modelProviderModelSearch")}
      emptyLabel={t("modelProviderNoModels")}
      options={[
        ...uniqueModels.map((model) => ({
          value: model,
          label: modelLabel(model),
          textValue: modelSearchText(model),
          description: isAuto(model) ? t("modelProviderAutoDescription") : undefined,
          separatorAfter: isAuto(model),
        })),
        ...(value && !uniqueModels.includes(value)
          ? [
              {
                value,
                label: modelLabel(value),
                textValue: modelSearchText(value),
                disabled: true,
                description: unavailableLabel ?? t("profileModelCurrentSelectionRetained"),
              },
            ]
          : []),
      ]}
    />
  );
}

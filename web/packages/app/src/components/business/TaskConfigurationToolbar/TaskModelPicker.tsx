import { useState } from "react";
import { Settings2 } from "lucide-react";
import { Button, Combobox } from "@/components/ui";
import { ModelOptionLabel } from "@/components/business/ProfileControls";
import { useOptionalWorkspaceControllerContext } from "@/hooks/workspace";
import { isOpenCSGAutoModel, type ModelProviderSelectOption } from "@/models/modelProviders";
import type { TranslateFn } from "@/models/conversations";
import styles from "./TaskConfigurationToolbar.module.css";

export function TaskModelPicker({
  providers,
  providerID,
  modelID,
  agentModelID,
  agentProviderID,
  followDefault,
  disabled,
  onChoose,
  onFollowDefault,
  t,
}: {
  providers: readonly ModelProviderSelectOption[];
  providerID: string;
  modelID: string;
  agentModelID: string;
  agentProviderID: string;
  followDefault: boolean;
  disabled: boolean;
  onChoose: (providerID: string, modelID: string) => Promise<void>;
  onFollowDefault: () => Promise<void>;
  t: TranslateFn;
}) {
  const controller = useOptionalWorkspaceControllerContext();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const current = providers.find((provider) => provider.id === providerID);
  const label = (id: string, model: string) => (isOpenCSGAutoModel(id, model) ? t("modelProviderAutoName") : model);
  const encode = (id: string, model: string) => JSON.stringify([id, model]);
  return (
    <Combobox
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        setError("");
      }}
      value={followDefault ? "" : encode(providerID, modelID)}
      selectedLabel={
        <ModelOptionLabel avatar={current?.avatar} model={label(providerID, modelID) || t("iaModelUnset")} />
      }
      disabled={disabled}
      size="sm"
      triggerClassName={styles.modelTrigger}
      triggerProps={{ "aria-label": t("iaModelTitle") }}
      contentClassName={styles.modelMenu}
      contentProps={{ side: "top", align: "end" }}
      searchPlaceholder={t("iaModelSearch")}
      emptyLabel={t("iaModelEmpty")}
      options={[
        {
          value: "",
          alwaysVisible: true,
          textValue: t("iaModelFollowAgent"),
          label: (
            <ModelOptionLabel
              avatar={providers.find((provider) => provider.id === agentProviderID)?.avatar}
              model={t("iaModelFollowAgent")}
              provider={agentModelID || t("iaModelFollowAgentHint")}
            />
          ),
        },
        ...providers
          .filter((provider) => !provider.disabled)
          .flatMap((provider) =>
            provider.models
              .filter((model) => !provider.imageModels?.includes(model) && !provider.videoModels?.includes(model))
              .map((model) => ({
                value: encode(provider.id, model),
                group: provider.displayName,
                textValue: `${model} ${provider.displayName}`,
                label: <ModelOptionLabel avatar={provider.avatar} model={label(provider.id, model)} />,
              })),
          ),
      ]}
      onValueChange={async (value) => {
        setError("");
        try {
          if (!value) await onFollowDefault();
          else {
            const [id, model] = JSON.parse(value) as [string, string];
            await onChoose(id, model);
          }
        } catch (cause) {
          setError(cause instanceof Error ? cause.message : t("sendFailed"));
          setOpen(true);
        }
      }}
      footer={
        <div className={styles.menuFooter}>
          {error ? (
            <p className={styles.error} role="alert">
              {error}
            </p>
          ) : null}
          {controller?.ready && controller.sidebarProps.onCreateModelProvider ? (
            <Button
              variant="tertiaryColor"
              size="sm"
              onClick={() => {
                setOpen(false);
                void controller.sidebarProps.onCreateModelProvider?.({
                  preset: "custom",
                  onConnected: () => setOpen(true),
                });
              }}
            >
              <Settings2 size={16} />
              {t("iaModelConfigure")}
            </Button>
          ) : null}
        </div>
      }
    />
  );
}

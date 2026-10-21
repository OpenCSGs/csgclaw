import type { ComboboxOption } from "@/components/ui/Combobox";
import { useState } from "react";
import { Button, Combobox } from "@/components/ui";
import { ModelOptionLabel } from "@/components/business/ProfileControls";
import { useOptionalWorkspaceControllerContext } from "@/hooks/workspace";
import { checkModelProvider } from "@/api/modelProviders";
import { errorMessage } from "@/api/client";
import { useModelProviderPresets } from "@/hooks/useModelProviderPresets";
import { isOpenCSGAutoModel, type ModelProviderSelectOption } from "@/models/modelProviders";
import type { TranslateFn } from "@/models/conversations";
import styles from "./ConnectedModelPicker.module.css";

export function ConnectedModelPicker({
  providers,
  providerID,
  modelID,
  disabled,
  followDefault,
  onChoose,
  onFollowDefault,
  t,
  compact = false,
  showConnectionActions = true,
  selectionLabel = "full",
  placeholder,
  footer,
}: {
  providers: readonly ModelProviderSelectOption[];
  providerID: string;
  modelID: string;
  disabled?: boolean;
  compact?: boolean;
  showConnectionActions?: boolean;
  selectionLabel?: "model" | "full";
  placeholder?: string;
  footer?: import("react").ReactNode;
  followDefault?: boolean;
  onChoose: (providerID: string, modelID: string) => void | Promise<void>;
  onFollowDefault?: () => void | Promise<void>;
  t: TranslateFn;
}) {
  const controller = useOptionalWorkspaceControllerContext();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [retrying, setRetrying] = useState("");
  const [showConnect, setShowConnect] = useState(false);
  const directory = useModelProviderPresets(showConnectionActions && showConnect);
  const encode = (p: string, m: string) => JSON.stringify([p, m]);
  const detail = (provider: ModelProviderSelectOption) =>
    provider.serviceName && provider.serviceName !== provider.displayName
      ? `${provider.serviceName} · ${provider.displayName}`
      : provider.displayName;
  const current = providers.find((provider) => provider.id === providerID);
  const providerGroups = new Map<string, ModelProviderSelectOption[]>();
  for (const provider of providers) {
    if (provider.disabled) continue;
    const name = provider.brandName || provider.displayName;
    const group = providerGroups.get(name) ?? [];
    group.push(provider);
    providerGroups.set(name, group);
  }
  const options: ComboboxOption[] = [...providerGroups].flatMap(([group, connections]) =>
    connections.flatMap((provider) =>
      provider.models
        .filter((model) => !provider.imageModels?.includes(model) && !provider.videoModels?.includes(model))
        .map((model) => ({
          value: encode(provider.id, model),
          group,
          textValue: `${model} ${detail(provider)} ${group} ${isOpenCSGAutoModel(provider.id, model) ? t("modelProviderAutoName") : ""}`,
          description: isOpenCSGAutoModel(provider.id, model) ? t("modelProviderAutoDescription") : undefined,
          label: (
            <ModelOptionLabel
              avatar={provider.avatar}
              model={isOpenCSGAutoModel(provider.id, model) ? t("modelProviderAutoName") : model}
              badge={isOpenCSGAutoModel(provider.id, model) ? t("modelProviderAutoBadge") : undefined}
              provider={detail(provider)}
            />
          ),
        })),
    ),
  );
  if (onFollowDefault)
    options.unshift({
      value: "",
      textValue: t("iaModelFollowAgent"),
      label: <ModelOptionLabel avatar={current?.avatar} model={t("iaModelFollowAgent")} provider={modelID} />,
      disabled: false,
    });
  const selected = followDefault || !modelID ? "" : encode(providerID, modelID);
  const connected = controller?.modelProviders?.providers ?? [];
  const failed = connected.filter((p) => p.status === "failed" && !p.builtin);
  async function choose(value: string) {
    setSaving(true);
    setError("");
    try {
      if (!value) await onFollowDefault?.();
      else {
        const [p, m] = JSON.parse(value) as [string, string];
        await onChoose(p, m);
      }
    } catch (cause) {
      setError(errorMessage(cause, t("sendFailed")));
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className={styles.root}>
      <Combobox
        value={selected}
        options={options}
        onValueChange={(value) => void choose(value)}
        disabled={disabled || saving}
        searchPlaceholder={t("modelProviderModelSearch")}
        emptyLabel={t("modelConnectNoMatch")}
        placeholder={placeholder ?? t(options.length ? "profileSelectModel" : "modelDefaultNone")}
        selectedLabel={
          followDefault ? (
            compact ? (
              modelID || t("iaModelFollowAgent")
            ) : (
              <ModelOptionLabel avatar={current?.avatar} model={t("iaModelFollowAgent")} />
            )
          ) : modelID ? (
            <ModelOptionLabel
              avatar={selectionLabel === "full" ? current?.avatar : undefined}
              model={isOpenCSGAutoModel(providerID, modelID) ? t("modelProviderAutoName") : modelID}
              provider={!compact && selectionLabel === "full" && current ? detail(current) : undefined}
            />
          ) : undefined
        }
        size={compact ? "sm" : "md"}
        triggerClassName={compact ? styles.compact : undefined}
        footer={
          <div className={styles.actions}>
            {showConnectionActions && controller?.ready ? (
              <Button
                size="sm"
                variant="tertiaryGray"
                disabled={disabled || saving}
                onClick={() => setShowConnect(!showConnect)}
              >
                {t("modelConnectAdd")}
              </Button>
            ) : null}
            {showConnectionActions && showConnect && controller?.ready ? (
              <div className={styles.services}>
                {(directory.data?.presets ?? [])
                  .filter((preset, index, all) => all.findIndex((p) => p.brand === preset.brand) === index)
                  .map((preset) => (
                    <Button
                      key={preset.brand}
                      size="sm"
                      variant="secondaryGray"
                      disabled={disabled || saving}
                      onClick={() => {
                        setShowConnect(false);
                        void controller.sidebarProps.onCreateModelProvider?.({
                          preset: preset.id,
                          onConnected: (provider) => {
                            if (provider.models[0]) void choose(encode(provider.id, provider.models[0]));
                          },
                        });
                      }}
                    >
                      <img src={preset.avatar} alt="" />
                      {preset.brand_name}
                    </Button>
                  ))}
                {directory.isError ? (
                  <Button size="sm" variant="secondaryGray" onClick={directory.refetch}>
                    {t("retry")}
                  </Button>
                ) : null}
              </div>
            ) : null}
            {showConnectionActions
              ? failed.map((provider) => (
                  <div key={provider.id} className={styles.retry}>
                    <span>{provider.display_name}</span>
                    <Button
                      variant="tertiaryGray"
                      size="sm"
                      disabled={disabled || Boolean(retrying)}
                      onClick={async () => {
                        setRetrying(provider.id);
                        setError("");
                        try {
                          const result = await checkModelProvider(provider.id, { connect: true });
                          if (result.status !== "connected") setError(result.message || t("modelProviderCheckFailed"));
                          await controller?.refreshWorkspaceModelProviders();
                        } catch (cause) {
                          setError(errorMessage(cause, t("modelProviderCheckFailed")));
                        } finally {
                          setRetrying("");
                        }
                      }}
                    >
                      {t("modelConnectRetry")}
                    </Button>
                  </div>
                ))
              : null}
            {showConnectionActions && error ? (
              <p className={styles.error} role="alert">
                {error}
              </p>
            ) : null}
            {footer}
          </div>
        }
        triggerProps={{ "aria-label": t(compact ? "iaModelTitle" : "profileModel") }}
      />
      {error ? (
        <p className={styles.error} role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}

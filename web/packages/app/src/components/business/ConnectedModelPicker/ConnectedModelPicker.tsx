import type { ComboboxOption } from "csgclaw-ui";
import { useId, useRef, useState } from "react";
import { Button, Combobox } from "@/components/ui";
import { ModelOptionLabel } from "@/components/business/ProfileControls";
import { errorMessage } from "@/api/client";
import { checkModelProvider } from "@/api/modelProviders";
import { useOptionalWorkspaceControllerContext } from "@/hooks/workspace";
import { useModelProviderPresets } from "@/hooks/useModelProviderPresets";
import { localizeAPIError } from "@/shared/i18n";
import {
  isOpenCSGAutoModel,
  modelProviderMessageLabel,
  type ModelProvider,
  type ModelProviderSelectOption,
} from "@/models/modelProviders";
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
  footer?: React.ReactNode;
  followDefault?: boolean;
  onChoose: (providerID: string, modelID: string) => void | Promise<void>;
  onFollowDefault?: () => void | Promise<void>;
  t: TranslateFn;
}) {
  const controller = useOptionalWorkspaceControllerContext();
  const addProvider =
    showConnectionActions && controller?.ready ? controller.sidebarProps.onCreateModelProvider : undefined;
  const failed = showConnectionActions
    ? (controller?.modelProviders?.providers ?? []).filter(
        (provider) => provider.status === "failed" && !provider.builtin,
      )
    : [];
  const [panel, setPanel] = useState<"add" | "retry" | null>(null);
  const hasConnectionActions = Boolean(addProvider || failed.length || panel === "retry");
  const [retrying, setRetrying] = useState("");
  const [checkFeedback, setCheckFeedback] = useState<{ providerID: string; message: string; failed: boolean } | null>(
    null,
  );
  const retryToggle = useRef<HTMLButtonElement>(null);
  const activeRetryButton = useRef<HTMLButtonElement | null>(null);
  const panelID = useId();
  const directory = useModelProviderPresets(Boolean(addProvider && panel === "add"));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
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
  async function retry(provider: ModelProvider) {
    setRetrying(provider.id);
    setCheckFeedback(null);
    try {
      const result = await checkModelProvider(provider.id, { connect: true });
      const connected = result.status === "connected";
      setCheckFeedback({
        providerID: provider.id,
        message: connected
          ? t("modelProviderConnected")
          : modelProviderMessageLabel(result, t) || t("modelProviderCheckFailed"),
        failed: !connected,
      });
      if (connected && activeRetryButton.current === document.activeElement) {
        retryToggle.current?.focus();
      }
      await controller?.refreshWorkspaceModelProviders();
    } catch (cause) {
      setCheckFeedback({
        providerID: provider.id,
        message: localizeAPIError(cause, t, t("modelProviderCheckFailed")),
        failed: true,
      });
    } finally {
      setRetrying("");
    }
  }

  return (
    <div className={styles.root}>
      <Combobox
        value={selected}
        options={options}
        onValueChange={(value) => void choose(value)}
        disabled={disabled || saving || (!options.length && !hasConnectionActions)}
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
        onOpenChange={(open) => {
          if (!open) setPanel(null);
        }}
        tabIntoFooter
        footer={
          hasConnectionActions || footer ? (
            <>
              {hasConnectionActions ? (
                <div className={styles.actions}>
                  <div className={styles.shortcuts}>
                    {addProvider ? (
                      <Button
                        size="sm"
                        variant="tertiaryGray"
                        aria-expanded={panel === "add"}
                        aria-controls={panel === "add" ? panelID : undefined}
                        onClick={() => setPanel(panel === "add" ? null : "add")}
                      >
                        {t("modelConnectAdd")}
                      </Button>
                    ) : null}
                    {failed.length > 0 || panel === "retry" ? (
                      <Button
                        ref={retryToggle}
                        size="sm"
                        variant="tertiaryGray"
                        aria-expanded={panel === "retry"}
                        aria-controls={panel === "retry" ? panelID : undefined}
                        onClick={() => setPanel(panel === "retry" ? null : "retry")}
                      >
                        {t("modelConnectRetryFailed", { count: failed.length })}
                      </Button>
                    ) : null}
                  </div>
                  {panel === "add" && addProvider ? (
                    <div id={panelID} className={styles.panel}>
                      <div className={styles.services}>
                        {(directory.data?.presets ?? [])
                          .filter((preset, index, all) => all.findIndex((p) => p.brand === preset.brand) === index)
                          .map((preset) => (
                            <Button
                              key={preset.brand}
                              size="sm"
                              variant="secondaryGray"
                              className={styles.service}
                              title={preset.brand_name}
                              aria-haspopup="dialog"
                              onClick={() => {
                                void addProvider({
                                  preset: preset.id,
                                  onConnected: (provider) => {
                                    const model = provider.models.find(
                                      (id) =>
                                        !provider.imageModels?.includes(id) && !provider.videoModels?.includes(id),
                                    );
                                    if (model) void choose(encode(provider.id, model));
                                  },
                                });
                              }}
                            >
                              <img src={preset.avatar} alt="" />
                              <span className={styles.serviceName}>{preset.brand_name}</span>
                            </Button>
                          ))}
                        {directory.isPending ? <span role="status">{t("loading")}</span> : null}
                        {directory.isError ? (
                          <Button size="sm" variant="secondaryGray" onClick={() => void directory.refetch()}>
                            {t("retry")}
                          </Button>
                        ) : null}
                      </div>
                    </div>
                  ) : null}
                  {panel === "retry" ? (
                    <div id={panelID} className={styles.panel}>
                      {failed.map((provider) => (
                        <div key={provider.id} className={styles.connection}>
                          <div className={styles.retry}>
                            <span className={styles.providerName}>{provider.display_name}</span>
                            <Button
                              size="sm"
                              variant="tertiaryGray"
                              aria-busy={retrying === provider.id}
                              aria-disabled={retrying === provider.id}
                              disabled={Boolean(retrying) && retrying !== provider.id}
                              aria-label={t("modelConnectRetryProvider", { name: provider.display_name })}
                              onClick={(event) => {
                                if (retrying) return;
                                activeRetryButton.current = event.currentTarget;
                                void retry(provider);
                              }}
                            >
                              {t(retrying === provider.id ? "loading" : "modelConnectRetry")}
                            </Button>
                          </div>
                          {checkFeedback?.failed && checkFeedback.providerID === provider.id ? (
                            <p className={styles.error} role="alert">
                              {checkFeedback.message}
                            </p>
                          ) : null}
                        </div>
                      ))}
                      {checkFeedback && !checkFeedback.failed ? (
                        <p className={styles.feedback} role="status">
                          {checkFeedback.message}
                        </p>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              ) : null}
              {footer}
            </>
          ) : undefined
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

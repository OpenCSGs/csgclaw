import { useEffect, useRef, useState } from "react";
import type { ModelProviderCheckResult, ModelProviderPayload } from "@/api/modelProviders";
import { ModelProviderModelList } from "@/components/business/ProfileControls";
import { Button, Select } from "@/components/ui";
import {
  MODEL_PROVIDER_PRESETS,
  modelProviderPresetMeta,
  normalizeModelProviderPreset,
} from "@/models/modelProviderPresets";
import {
  modelProviderAvatarPath,
  modelProviderDisplayNameExists,
  parseModelProviderModelsText,
  type ModelProviderCatalog,
} from "@/models/modelProviders";
import type { TranslateFn } from "@/models/conversations";
import { ModalCloseButton } from "./ModalCloseButton";

const INITIAL_PRESET: keyof typeof MODEL_PROVIDER_PRESETS = "openai";

export type CreateModelProviderPayload = Pick<
  ModelProviderPayload,
  "api_key" | "base_url" | "display_name" | "models" | "preset"
>;

export type CreateModelProviderModalProps = {
  busy: boolean;
  error?: string;
  modelProviders?: ModelProviderCatalog | null;
  onCheckAccess?: (payload: CreateModelProviderPayload) => Promise<ModelProviderCheckResult> | ModelProviderCheckResult;
  onClose: () => void;
  onCreate: (payload: CreateModelProviderPayload) => Promise<void> | void;
  t: TranslateFn;
};

function requiredLabel(label: string) {
  return (
    <span>
      {label} <span className="field-required-star">*</span>
    </span>
  );
}

export function CreateModelProviderModal({
  busy,
  error = "",
  modelProviders = null,
  onCheckAccess,
  onClose,
  onCreate,
  t,
}: CreateModelProviderModalProps) {
  const initialPresetMeta = modelProviderPresetMeta(INITIAL_PRESET);
  const [preset, setPreset] = useState<keyof typeof MODEL_PROVIDER_PRESETS>(INITIAL_PRESET);
  const [displayName, setDisplayName] = useState("");
  const [baseURL, setBaseURL] = useState<string>(initialPresetMeta.defaultBaseURL);
  const [apiKey, setAPIKey] = useState("");
  const [apiKeyVisible, setAPIKeyVisible] = useState(false);
  const [modelsText, setModelsText] = useState("");
  const [autoCheckBusy, setAutoCheckBusy] = useState(false);
  const [autoCheckMessage, setAutoCheckMessage] = useState("");
  const [autoCheckWarning, setAutoCheckWarning] = useState("");
  const [checkState, setCheckState] = useState<"idle" | "checking" | "success" | "empty" | "error">("idle");
  const [isBodyScrolling, setIsBodyScrolling] = useState(false);
  const bodyScrollTimerRef = useRef<number | null>(null);
  const trimmedName = displayName.trim();
  const trimmedBaseURL = baseURL.trim();
  const trimmedAPIKey = apiKey.trim();
  const modelList = parseModelProviderModelsText(modelsText);
  const duplicateName = modelProviderDisplayNameExists(modelProviders, trimmedName);
  const saveDisabled = busy || !trimmedName || !trimmedBaseURL || duplicateName;
  const checkDisabled = autoCheckBusy || busy || !onCheckAccess || !trimmedBaseURL || !trimmedAPIKey;
  const presetMeta = modelProviderPresetMeta(preset);
  const isDirty =
    preset !== INITIAL_PRESET ||
    trimmedName !== "" ||
    trimmedBaseURL !== initialPresetMeta.defaultBaseURL ||
    trimmedAPIKey !== "" ||
    modelsText.trim() !== "";

  useEffect(() => {
    setPreset(INITIAL_PRESET);
    setDisplayName("");
    setBaseURL(initialPresetMeta.defaultBaseURL);
    setAPIKey("");
    setAPIKeyVisible(false);
    setModelsText("");
    setAutoCheckBusy(false);
    setAutoCheckMessage("");
    setAutoCheckWarning("");
    setCheckState("idle");
  }, [initialPresetMeta.defaultBaseURL]);

  useEffect(
    () => () => {
      if (bodyScrollTimerRef.current) {
        window.clearTimeout(bodyScrollTimerRef.current);
      }
    },
    [],
  );

  useEffect(() => {
    setAutoCheckMessage("");
    setAutoCheckWarning("");
    setCheckState("idle");
  }, [preset, trimmedAPIKey, trimmedBaseURL]);

  function handleCreate() {
    if (saveDisabled) {
      return;
    }
    void onCreate({
      api_key: trimmedAPIKey || undefined,
      base_url: trimmedBaseURL,
      display_name: trimmedName,
      models: modelList,
      preset,
    });
  }

  function requestClose() {
    if (busy || autoCheckBusy) {
      return;
    }
    if (isDirty && !window.confirm(t("modelProviderDiscardConfirm"))) {
      return;
    }
    onClose();
  }

  async function handleCheckAccess() {
    if (checkDisabled || !onCheckAccess) {
      return;
    }
    setAutoCheckBusy(true);
    setAutoCheckMessage("");
    setAutoCheckWarning("");
    setCheckState("checking");
    try {
      const result = await Promise.resolve(
        onCheckAccess({
          api_key: trimmedAPIKey,
          base_url: trimmedBaseURL,
          display_name: trimmedName,
          models: [],
          preset,
        }),
      );
      if (result.status === "connected") {
        if (result.models?.length) {
          setModelsText(result.models.join("\n"));
          setCheckState("success");
        } else {
          setCheckState("empty");
        }
        setAutoCheckMessage(result.message || t("modelProviderConnected"));
        return;
      }
      setCheckState("error");
      setAutoCheckWarning(result.message || result.status || "");
    } catch (err) {
      setCheckState("error");
      setAutoCheckWarning(err instanceof Error ? err.message : String(err || ""));
    } finally {
      setAutoCheckBusy(false);
    }
  }

  function handlePresetChange(value: string) {
    const nextPreset = normalizeModelProviderPreset(value);
    const nextMeta = modelProviderPresetMeta(nextPreset);
    const currentMeta = modelProviderPresetMeta(preset);
    const shouldReplaceName = !trimmedName || trimmedName === currentMeta.defaultDisplayName;
    const shouldReplaceBaseURL = !trimmedBaseURL || trimmedBaseURL === currentMeta.defaultBaseURL;
    const shouldClearModels = modelsText.trim() === "";
    setPreset(nextPreset);
    if (shouldReplaceName) {
      setDisplayName(nextMeta.defaultDisplayName);
    }
    if (shouldReplaceBaseURL) {
      setBaseURL(nextMeta.defaultBaseURL);
    }
    if (shouldClearModels) {
      setModelsText("");
    }
    setAutoCheckMessage("");
    setAutoCheckWarning("");
    setCheckState("idle");
  }

  function onBodyScroll() {
    setIsBodyScrolling(true);
    if (bodyScrollTimerRef.current) {
      window.clearTimeout(bodyScrollTimerRef.current);
    }
    bodyScrollTimerRef.current = window.setTimeout(() => {
      setIsBodyScrolling(false);
      bodyScrollTimerRef.current = null;
    }, 700);
  }

  const modelStatusHint =
    checkState === "checking"
      ? t("profileLoadingModels")
      : checkState === "success"
        ? t("modelProviderModelCount", { count: modelList.length })
        : checkState === "empty"
          ? t("modelProviderCheckEmpty")
          : checkState === "error"
            ? autoCheckWarning || t("modelProviderCheckFailed")
            : t("modelProviderNotChecked");

  return (
    <div className="modal-backdrop" onClick={requestClose}>
      <div className="modal-card create-model-provider-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("modelProviderCreateTitle")}</div>
            <div className="create-model-provider-subtitle">{t("modelProviderCreateSubtitle")}</div>
            {error ? (
              <div role="alert" className="form-error create-model-provider-submit-error">
                {error}
              </div>
            ) : null}
          </div>
          <ModalCloseButton label={t("close")} onClose={requestClose} />
        </div>
        <div className={`create-model-provider-body${isBodyScrolling ? " is-scrolling" : ""}`} onScroll={onBodyScroll}>
          <section className="create-model-provider-section">
            <div className="create-model-provider-section-heading">
              <div>{t("modelProviderCreatePresetTitle")}</div>
              <p>{t("modelProviderCreatePresetDescription")}</p>
            </div>
            <div className="create-model-provider-section-form">
              <label className="field">
                {requiredLabel(t("modelProviderPreset"))}
                <Select
                  value={preset}
                  onValueChange={handlePresetChange}
                  triggerProps={{ "aria-label": t("modelProviderPreset"), "aria-required": true }}
                  options={[
                    { value: "openai", label: t("modelProviderPresetOpenAI") },
                    { value: "zhipu", label: t("modelProviderPresetZhipu") },
                    { value: "deepseek", label: t("modelProviderPresetDeepSeek") },
                    { value: "custom", label: t("modelProviderPresetCustom") },
                  ]}
                />
              </label>
            </div>
          </section>

          <section className="create-model-provider-section">
            <div className="create-model-provider-section-heading">
              <div>{t("modelProviderCreateIdentityTitle")}</div>
              <p>{t("modelProviderCreateIdentityDescription")}</p>
            </div>
            <div className="create-model-provider-section-form">
              <div className="create-model-provider-identity">
                <div className="field create-model-provider-avatar-field">
                  <span>{t("modelProviderAvatar")}</span>
                  <div className="create-model-provider-avatar" aria-hidden="true">
                    <img src={presetMeta.avatar || modelProviderAvatarPath("openai")} alt="" />
                  </div>
                </div>
                <label className="field">
                  {requiredLabel(t("modelProviderDisplayName"))}
                  <input
                    aria-invalid={duplicateName || undefined}
                    value={displayName}
                    autoFocus
                    onInput={(event) => setDisplayName(event.currentTarget.value)}
                    placeholder={presetMeta.defaultDisplayName}
                  />
                  {duplicateName ? (
                    <small className="form-error">{t("modelProviderDuplicateDisplayName")}</small>
                  ) : null}
                </label>
              </div>
            </div>
          </section>

          <section className="create-model-provider-section">
            <div className="create-model-provider-section-heading">
              <div>{t("modelProviderCreateConnectionTitle")}</div>
              <p>{t("modelProviderCreateConnectionDescription")}</p>
            </div>
            <div className="create-model-provider-section-form">
              <div className="create-model-provider-grid">
                <label className="field create-model-provider-span-2">
                  {requiredLabel(t("profileBaseURL"))}
                  <input
                    aria-invalid={!trimmedBaseURL || undefined}
                    value={baseURL}
                    onInput={(event) => setBaseURL(event.currentTarget.value)}
                    placeholder={presetMeta.defaultBaseURL}
                  />
                  {!trimmedBaseURL ? <small className="form-error">{t("modelProviderBaseURLRequired")}</small> : null}
                </label>
                <label className="field create-model-provider-span-2">
                  <span>{t("profileAPIKey")}</span>
                  <div className="create-model-provider-secret-row">
                    <input
                      className="create-model-provider-secret-input"
                      value={apiKey}
                      type={apiKeyVisible ? "text" : "password"}
                      onInput={(event) => setAPIKey(event.currentTarget.value)}
                      placeholder={t("profileAPIKeyNewPlaceholder")}
                    />
                    <button
                      type="button"
                      className="create-model-provider-secret-toggle"
                      aria-label={apiKeyVisible ? t("modelProviderHideSecret") : t("modelProviderShowSecret")}
                      aria-pressed={apiKeyVisible}
                      onClick={() => setAPIKeyVisible((current) => !current)}
                    >
                      <img src={apiKeyVisible ? "icons/eye-off.svg" : "icons/eye.svg"} alt="" aria-hidden="true" />
                    </button>
                  </div>
                  <small className="field-hint">{t("modelProviderAPIKeyHint")}</small>
                </label>
                {onCheckAccess ? (
                  <div className="field create-model-provider-span-2">
                    <div className="create-model-provider-check-row">
                      <Button variant="secondaryGray" size="sm" disabled={checkDisabled} onClick={handleCheckAccess}>
                        {autoCheckBusy ? t("profileLoadingModels") : t("modelProviderCheck")}
                      </Button>
                      {autoCheckMessage ? (
                        <span className="create-model-provider-check-status success">{autoCheckMessage}</span>
                      ) : null}
                      {autoCheckWarning ? (
                        <span className="create-model-provider-check-status warning">{autoCheckWarning}</span>
                      ) : null}
                    </div>
                    {!trimmedBaseURL || !trimmedAPIKey ? (
                      <small className="field-hint">{t("modelProviderCheckRequiresConnection")}</small>
                    ) : null}
                  </div>
                ) : null}
                <div className="field create-model-provider-span-2">
                  <span>{t("modelProviderModels")}</span>
                  <ModelProviderModelList
                    className="create-model-provider-model-list"
                    emptyLabel={checkState === "idle" ? t("modelProviderNotChecked") : t("modelProviderNoModels")}
                    modelListLabel={t("modelProviderModels")}
                    models={modelList}
                    searchLabel={t("modelProviderModelSearch")}
                  />
                  <small
                    className={`field-hint create-model-provider-model-status ${checkState === "error" ? "warning" : ""}`.trim()}
                  >
                    {modelStatusHint}
                  </small>
                </div>
              </div>
            </div>
          </section>

          {autoCheckMessage ? <div className="model-provider-save-status">{autoCheckMessage}</div> : null}
        </div>
        <div className="modal-actions">
          <Button variant="secondaryGray" size="md" disabled={busy || autoCheckBusy} onClick={requestClose}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            size="md"
            loading={busy}
            disabled={saveDisabled || autoCheckBusy}
            onClick={handleCreate}
          >
            {t("modelProviderCreateAction")}
          </Button>
        </div>
      </div>
    </div>
  );
}

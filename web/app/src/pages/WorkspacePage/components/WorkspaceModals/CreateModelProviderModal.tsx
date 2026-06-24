import { useEffect, useRef, useState } from "react";
import type { ModelProviderCheckResult, ModelProviderPayload } from "@/api/modelProviders";
import { ModelProviderModelList } from "@/components/business/ProfileControls";
import { Button } from "@/components/ui";
import {
  modelProviderAvatarPath,
  modelProviderDisplayNameExists,
  parseModelProviderModelsText,
  type ModelProviderCatalog,
} from "@/models/modelProviders";
import type { TranslateFn } from "@/models/conversations";
import { ModalCloseButton } from "./ModalCloseButton";

const DEFAULT_OPENAI_BASE_URL = "https://api.openai.com/v1";

export type CreateModelProviderPayload = Pick<ModelProviderPayload, "api_key" | "base_url" | "display_name" | "models">;

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
  const [displayName, setDisplayName] = useState("");
  const [baseURL, setBaseURL] = useState(DEFAULT_OPENAI_BASE_URL);
  const [apiKey, setAPIKey] = useState("");
  const [modelsText, setModelsText] = useState("");
  const [autoCheckBusy, setAutoCheckBusy] = useState(false);
  const [autoCheckMessage, setAutoCheckMessage] = useState("");
  const [autoCheckWarning, setAutoCheckWarning] = useState("");
  const [isBodyScrolling, setIsBodyScrolling] = useState(false);
  const bodyScrollTimerRef = useRef<number | null>(null);
  const trimmedName = displayName.trim();
  const trimmedBaseURL = baseURL.trim();
  const trimmedAPIKey = apiKey.trim();
  const modelList = parseModelProviderModelsText(modelsText);
  const duplicateName = modelProviderDisplayNameExists(modelProviders, trimmedName);
  const saveDisabled = busy || !trimmedName || !trimmedBaseURL || duplicateName;

  useEffect(() => {
    setDisplayName("");
    setBaseURL(DEFAULT_OPENAI_BASE_URL);
    setAPIKey("");
    setModelsText("");
    setAutoCheckBusy(false);
    setAutoCheckMessage("");
    setAutoCheckWarning("");
  }, []);

  useEffect(
    () => () => {
      if (bodyScrollTimerRef.current) {
        window.clearTimeout(bodyScrollTimerRef.current);
      }
    },
    [],
  );

  useEffect(() => {
    if (!onCheckAccess || !trimmedBaseURL || !trimmedAPIKey) {
      setAutoCheckBusy(false);
      setAutoCheckMessage("");
      setAutoCheckWarning("");
      return undefined;
    }
    let cancelled = false;
    setAutoCheckBusy(true);
    setAutoCheckMessage("");
    setAutoCheckWarning("");
    const timeout = window.setTimeout(() => {
      void Promise.resolve(
        onCheckAccess({
          api_key: trimmedAPIKey,
          base_url: trimmedBaseURL,
          display_name: trimmedName,
          models: [],
        }),
      )
        .then((result) => {
          if (cancelled) {
            return;
          }
          if (result.status === "connected" && result.models?.length) {
            setModelsText(result.models.join("\n"));
            setAutoCheckMessage(result.message || "connected");
            return;
          }
          setAutoCheckWarning(result.message || result.status || "");
        })
        .catch((err) => {
          if (!cancelled) {
            setAutoCheckWarning(err instanceof Error ? err.message : String(err || ""));
          }
        })
        .finally(() => {
          if (!cancelled) {
            setAutoCheckBusy(false);
          }
        });
    }, 450);
    return () => {
      cancelled = true;
      window.clearTimeout(timeout);
    };
  }, [onCheckAccess, trimmedAPIKey, trimmedBaseURL, trimmedName]);

  function handleCreate() {
    if (saveDisabled) {
      return;
    }
    void onCreate({
      api_key: trimmedAPIKey || undefined,
      base_url: trimmedBaseURL,
      display_name: trimmedName,
      models: modelList,
    });
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

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-card create-model-provider-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">{t("modelProviderCreateTitle")}</div>
            {autoCheckWarning ? (
              <div role="alert" className="form-error create-model-provider-check-error">
                {autoCheckWarning}
              </div>
            ) : null}
            {error ? (
              <div role="alert" className="form-error create-model-provider-submit-error">
                {error}
              </div>
            ) : null}
          </div>
          <ModalCloseButton label={t("close")} onClose={onClose} />
        </div>
        <div className={`create-model-provider-body${isBodyScrolling ? " is-scrolling" : ""}`} onScroll={onBodyScroll}>
          <section className="create-model-provider-section">
            <div className="create-model-provider-section-heading">
              <div>{t("modelProviderCreateIdentityTitle")}</div>
              <p>{t("modelProviderCreateIdentityDescription")}</p>
            </div>
            <div className="create-model-provider-identity">
              <div className="field create-model-provider-avatar-field">
                <span>{t("modelProviderAvatar")}</span>
                <div className="create-model-provider-avatar" aria-hidden="true">
                  <img src={modelProviderAvatarPath("openai")} alt="" />
                </div>
              </div>
              <label className="field">
                {requiredLabel(t("modelProviderDisplayName"))}
                <input
                  value={displayName}
                  autoFocus
                  onInput={(event) => setDisplayName(event.currentTarget.value)}
                  placeholder="OpenAI API"
                />
              </label>
            </div>
          </section>

          <section className="create-model-provider-section">
            <div className="create-model-provider-section-heading">
              <div>{t("modelProviderCreateConnectionTitle")}</div>
              <p>{t("modelProviderCreateConnectionDescription")}</p>
            </div>
            <div className="create-model-provider-grid">
              <label className="field create-model-provider-span-2">
                {requiredLabel(t("profileBaseURL"))}
                <input
                  value={baseURL}
                  onInput={(event) => setBaseURL(event.currentTarget.value)}
                  placeholder={DEFAULT_OPENAI_BASE_URL}
                />
              </label>
              <label className="field create-model-provider-span-2">
                <span>{t("profileAPIKey")}</span>
                <input
                  value={apiKey}
                  type="text"
                  onInput={(event) => setAPIKey(event.currentTarget.value)}
                  placeholder={t("profileAPIKeyNewPlaceholder")}
                />
                <small className="field-hint">{t("modelProviderAPIKeyHint")}</small>
              </label>
              <div className="field create-model-provider-span-2">
                <span>{t("modelProviderModels")}</span>
                <ModelProviderModelList
                  className="create-model-provider-model-list"
                  emptyLabel={t("modelProviderNoModels")}
                  modelListLabel={t("modelProviderModels")}
                  models={modelList}
                  searchLabel={t("modelProviderModelSearch")}
                />
                <small className="field-hint">
                  {autoCheckBusy
                    ? t("profileLoadingModels")
                    : modelList.length
                      ? t("modelProviderModelCount", { count: modelList.length })
                      : t("modelProviderModelsHint")}
                </small>
              </div>
            </div>
          </section>

          {duplicateName ? <div className="form-warning">{t("modelProviderDuplicateDisplayName")}</div> : null}
          {!trimmedBaseURL ? <div className="form-warning">{t("modelProviderBaseURLRequired")}</div> : null}
          {autoCheckMessage ? <div className="model-provider-save-status">{autoCheckMessage}</div> : null}
        </div>
        <div className="modal-actions">
          <Button variant="secondaryGray" size="md" disabled={busy} onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" size="md" loading={busy} disabled={saveDisabled} onClick={handleCreate}>
            {t("modelProviderCreateAction")}
          </Button>
        </div>
      </div>
    </div>
  );
}

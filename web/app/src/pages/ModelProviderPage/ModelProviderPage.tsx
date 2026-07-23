import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertCircle, CheckCircle2, LogIn, RefreshCw, Save, Trash2 } from "lucide-react";
import { errorMessage } from "@/api/client";
import { checkModelProvider, deleteModelProvider, updateModelProvider } from "@/api/modelProviders";
import { APIKeyField, ModelProviderModelList } from "@/components/business/ProfileControls";
import {
  Button,
  DialogCloseButton,
  DialogContent,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Tooltip,
} from "@/components/ui";
import { useWorkspaceControllerContext } from "@/hooks/workspace";
import { isAuthenticated } from "@/models/auth";
import {
  modelProviderAvatarPath,
  parseModelProviderModelsText,
  providerStatusTone,
  type ModelProvider,
} from "@/models/modelProviders";
import { WorkspacePaneTypes } from "@/models/routing";
import "./ModelProviderPage.css";

type ProviderDraft = {
  apiKey: string;
  baseURL: string;
  displayName: string;
  modelsText: string;
};

type ProviderCheckState = {
  lastCheckedAt?: string;
  message?: string;
  status: string;
};

function providerToDraft(provider: ModelProvider | null | undefined): ProviderDraft {
  return {
    apiKey: "",
    baseURL: provider?.base_url || "",
    displayName: provider?.display_name || provider?.id || "",
    modelsText: (provider?.models || []).join("\n"),
  };
}

function providerToCheckState(provider: ModelProvider | null | undefined): ProviderCheckState {
  return {
    lastCheckedAt: provider?.last_checked_at,
    message: provider?.message,
    status: provider?.status || "unknown",
  };
}

export function ModelProviderPage() {
  const controller = useWorkspaceControllerContext();
  const { activePane, modelProviders, refreshWorkspaceModelProviders, t } = controller;
  const providerID = activePane.type === WorkspacePaneTypes.modelProvider ? String(activePane.id || "") : "";
  const provider = useMemo(
    () => modelProviders?.providers.find((item) => item.id === providerID) ?? null,
    [modelProviders?.providers, providerID],
  );
  const draftSourceKey = provider ? `provider:${provider.id}` : `missing:${providerID}`;
  const draftSourceKeyRef = useRef(draftSourceKey);
  const providerBaseURL = provider?.base_url || "";
  const providerLastCheckedAt = provider?.last_checked_at;
  const providerMessage = provider?.message;
  const providerStatus = provider?.status || "unknown";
  const [draft, setDraft] = useState<ProviderDraft>(() => providerToDraft(provider));
  const [checkState, setCheckState] = useState<ProviderCheckState>(() => providerToCheckState(provider));
  const [busy, setBusy] = useState("");
  const [saveStatus, setSaveStatus] = useState("");
  const [error, setError] = useState("");
  const [deleteSuccess, setDeleteSuccess] = useState(false);
  const isBuiltinCLI = provider?.id === "codex" || provider?.id === "claude_code";
  const isOpenCSG = provider?.id === "opencsg";
  const canEditEndpoint = Boolean(provider && !isBuiltinCLI && !isOpenCSG);
  const opencsgAuthenticatedForCheck = controller.ready && isAuthenticated(controller.sidebarProps?.authStatus);
  const opencsgModelsLoaded = isOpenCSG && providerStatus === "connected" && Boolean(provider?.models.length);

  const runCheckForDraft = useCallback(
    async (baseURL: string, apiKey: string, options: { showError?: boolean } = {}) => {
      if (!providerID) {
        return;
      }
      if (options.showError) {
        setBusy("check");
        setError("");
        setSaveStatus("");
      }
      try {
        const result = await checkModelProvider(providerID, {
          base_url: baseURL,
          api_key: apiKey,
        });
        setCheckState({
          lastCheckedAt: result.last_checked_at,
          message: result.message || result.status,
          status: result.status,
        });
        if (result.models?.length) {
          setDraft((current) => ({ ...current, modelsText: result.models.join("\n") }));
        }
        await refreshWorkspaceModelProviders();
      } catch (err) {
        if (options.showError) {
          setError(errorMessage(err, "Check failed"));
        }
      } finally {
        if (options.showError) {
          setBusy("");
        }
      }
    },
    [providerID, refreshWorkspaceModelProviders],
  );

  useEffect(() => {
    if (draftSourceKeyRef.current === draftSourceKey) {
      return;
    }
    draftSourceKeyRef.current = draftSourceKey;
    setDraft(providerToDraft(provider));
    setSaveStatus("");
    setError("");
  }, [draftSourceKey, provider]);

  useEffect(() => {
    if (!isOpenCSG) {
      return;
    }
    setDraft(providerToDraft(provider));
  }, [isOpenCSG, provider]);

  useEffect(() => {
    setCheckState({
      lastCheckedAt: providerLastCheckedAt,
      message: providerMessage,
      status: providerStatus,
    });
  }, [providerLastCheckedAt, providerMessage, providerStatus]);

  useEffect(() => {
    if (!providerID) {
      return;
    }
    if (isOpenCSG && (!opencsgAuthenticatedForCheck || opencsgModelsLoaded)) {
      return;
    }
    void runCheckForDraft(providerBaseURL, "", { showError: true });
  }, [isOpenCSG, opencsgAuthenticatedForCheck, opencsgModelsLoaded, providerBaseURL, providerID, runCheckForDraft]);

  useEffect(() => {
    if (!providerID || !canEditEndpoint) {
      return undefined;
    }
    const baseURL = draft.baseURL.trim();
    const apiKey = draft.apiKey.trim();
    if (!baseURL || (!apiKey && !provider?.api_key_set)) {
      return undefined;
    }
    if (baseURL === providerBaseURL && !apiKey) {
      return undefined;
    }
    const timeout = window.setTimeout(() => {
      void runCheckForDraft(baseURL, apiKey);
    }, 600);
    return () => window.clearTimeout(timeout);
  }, [
    canEditEndpoint,
    draft.apiKey,
    draft.baseURL,
    provider?.api_key_set,
    providerBaseURL,
    providerID,
    runCheckForDraft,
  ]);

  if (!controller.ready) {
    return null;
  }

  if (!provider) {
    return (
      <section className="model-provider-page empty">
        <div className="empty-state">{t("profileSelectModel")}</div>
      </section>
    );
  }

  const authStatus = controller.sidebarProps?.authStatus ?? null;
  const authBusy = Boolean(controller.sidebarProps?.authBusy);
  const authPending = Boolean(controller.sidebarProps?.authPending);
  const opencsgSignedIn = isOpenCSG ? isAuthenticated(authStatus) : true;
  const effectiveTone = isOpenCSG && !opencsgSignedIn ? "warning" : providerStatusTone(checkState.status, provider);
  const providerSubtitle = isBuiltinCLI ? provider.kind : provider.base_url || draft.baseURL || provider.kind;
  const modelList = parseModelProviderModelsText(draft.modelsText);
  const showOpenCSGSignIn = isOpenCSG && !opencsgSignedIn;
  const checkMessage =
    showOpenCSGSignIn || !checkState.status
      ? ""
      : checkState.message ||
        (checkState.status === "connected" ? t("modelProviderConnected") : t("modelProviderCheck"));
  const statusLabel =
    isOpenCSG && !opencsgSignedIn
      ? t("csghubNotSignedIn")
      : checkState.status === "connected" || effectiveTone === "online"
        ? t("modelProviderConnected")
        : checkState.status;

  async function runCheck() {
    await runCheckForDraft(draft.baseURL, draft.apiKey, { showError: true });
  }

  async function saveProvider() {
    if (!provider) {
      return;
    }
    setBusy("save");
    setError("");
    setSaveStatus("");
    try {
      await updateModelProvider(provider.id, {
        display_name: provider.builtin ? undefined : draft.displayName,
        base_url: draft.baseURL,
        api_key: draft.apiKey,
        models: parseModelProviderModelsText(draft.modelsText),
      });
      await refreshWorkspaceModelProviders();
      setSaveStatus(t("profileSavedToast"));
    } catch (err) {
      setError(errorMessage(err, "Save failed"));
    } finally {
      setBusy("");
    }
  }

  async function removeProvider() {
    if (!provider || provider.builtin) {
      return;
    }
    setBusy("delete");
    setError("");
    setSaveStatus("");
    try {
      await deleteModelProvider(provider.id);
      setDeleteSuccess(true);
    } catch (err) {
      setError(errorMessage(err, "Delete failed"));
    } finally {
      setBusy("");
    }
  }

  function dismissDeleteSuccess() {
    setDeleteSuccess(false);
    void refreshWorkspaceModelProviders();
    controller.sidebarProps?.onSelectComputer?.();
  }

  return (
    <Fragment>
      <section className="model-provider-page">
        <header className="model-provider-header">
          <img
            className="model-provider-header-avatar"
            src={modelProviderAvatarPath(provider)}
            alt=""
            aria-hidden="true"
          />
          <div className="model-provider-header-main">
            <div className="model-provider-title-row">
              <h1>{provider.display_name || provider.id}</h1>
            </div>
            <p>{providerSubtitle}</p>
          </div>
          <div className={`model-provider-status-pill ${effectiveTone}`}>
            <span className={`workspace-status-dot ${effectiveTone}`} aria-hidden="true"></span>
            <span>{statusLabel}</span>
          </div>
          <div className="model-provider-actions">
            <Button variant="secondaryGray" onClick={runCheck} disabled={Boolean(busy)}>
              <RefreshCw size={16} aria-hidden="true" />
              {busy === "check" ? t("profileLoadingModels") : t("modelProviderCheck")}
            </Button>
            {!isOpenCSG ? (
              <Button variant="primary" onClick={saveProvider} disabled={Boolean(busy)}>
                <Save size={16} aria-hidden="true" />
                {busy === "save" ? t("profileLoadingModels") : t("agentUpdateSave")}
              </Button>
            ) : null}
            {showOpenCSGSignIn ? (
              <Button
                variant="primary"
                onClick={() => void controller.sidebarProps?.onLogin?.()}
                disabled={authBusy || authPending}
              >
                <LogIn size={16} aria-hidden="true" />
                {authPending ? t("csghubLoginPending") : t("csghubSignIn")}
              </Button>
            ) : null}
            {!provider.builtin ? (
              <Tooltip content={t("agentDelete")}>
                <span>
                  <Button
                    variant="outlineDanger"
                    aria-label={t("agentDelete")}
                    onClick={removeProvider}
                    disabled={Boolean(busy)}
                  >
                    <Trash2 size={16} aria-hidden="true" />
                  </Button>
                </span>
              </Tooltip>
            ) : null}
          </div>
        </header>

        {error ? <div className="form-error">{error}</div> : null}
        {saveStatus ? <div className="model-provider-save-status">{saveStatus}</div> : null}
        {showOpenCSGSignIn ? (
          <div className="model-provider-notice warning opencsg-signin-warning">
            <AlertCircle size={16} aria-hidden="true" />
            <span>{t("modelProviderOpenCSGSignInRequired")}</span>
          </div>
        ) : null}
        {checkMessage ? (
          <div className={`model-provider-notice ${effectiveTone === "warning" ? "warning" : "success"}`}>
            {effectiveTone === "warning" ? (
              <AlertCircle size={16} aria-hidden="true" />
            ) : (
              <CheckCircle2 size={16} aria-hidden="true" />
            )}
            <span>{checkMessage}</span>
          </div>
        ) : null}

        <div className="model-provider-grid">
          <section className="model-provider-card">
            <div className="model-provider-card-heading">
              <h2>{t("modelProviderConfiguration")}</h2>
              <p>
                {isOpenCSG
                  ? t("modelProviderOpenCSGSettings")
                  : provider.builtin
                    ? t("modelProviderBuiltinSettings")
                    : t("modelProviderCustomSettings")}
              </p>
            </div>
            {isOpenCSG ? (
              <div className="opencsg-gateway-panel">
                <div className="opencsg-gateway-address">
                  <span>{t("modelProviderAIGatewayAddress")}</span>
                  <code>{provider.base_url || draft.baseURL}</code>
                </div>
              </div>
            ) : (
              <div className="model-provider-form-grid">
                {!provider.builtin ? (
                  <label className="field">
                    <span>{t("agentName")}</span>
                    <input
                      value={draft.displayName}
                      onInput={(event) => {
                        const value = event.currentTarget.value;
                        setDraft((current) => ({ ...current, displayName: value }));
                      }}
                    />
                  </label>
                ) : null}
                {canEditEndpoint ? (
                  <>
                    <label className="field">
                      {t("profileBaseURL")}
                      <input
                        value={draft.baseURL}
                        onInput={(event) => {
                          const value = event.currentTarget.value;
                          setDraft((current) => ({ ...current, baseURL: value }));
                        }}
                        placeholder="https://api.openai.com/v1"
                      />
                    </label>
                    <APIKeyField
                      t={t}
                      value={draft.apiKey}
                      profile={provider}
                      unchangedHint={t("modelProviderStoredAPIKeyHint")}
                      onInput={(event) => {
                        const value = event.currentTarget.value;
                        setDraft((current) => ({ ...current, apiKey: value }));
                      }}
                    />
                  </>
                ) : null}
              </div>
            )}
          </section>

          <section className="model-provider-card model-provider-models-card">
            <div className="model-provider-card-heading">
              <h2>{t("profileModel")}</h2>
              <p>
                {modelList.length
                  ? t("modelProviderModelCount", { count: modelList.length })
                  : t("modelProviderNoModels")}
              </p>
            </div>
            <ModelProviderModelList
              emptyLabel={t("modelProviderNoModels")}
              modelListLabel={t("modelProviderModels")}
              models={modelList}
              searchLabel={t("modelProviderModelSearch")}
            />
          </section>
        </div>
      </section>

      <DialogRoot
        open={deleteSuccess}
        onOpenChange={(open) => {
          if (!open) {
            dismissDeleteSuccess();
          }
        }}
      >
        <DialogContent
          className="model-provider-delete-success-dialog"
          overlayClassName="model-provider-delete-success-backdrop"
        >
          <DialogHeader className="model-provider-delete-success-header">
            <div className="model-provider-delete-success-copy">
              <DialogTitle>{t("modelProviderDeleteSuccess")}</DialogTitle>
            </div>
            <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" />
          </DialogHeader>
          <div className="model-provider-delete-success-actions">
            <Button
              className="model-provider-delete-success-button"
              variant="primary"
              size="sm"
              onClick={dismissDeleteSuccess}
            >
              {t("modelProviderDeleteSuccessDismiss")}
            </Button>
          </div>
        </DialogContent>
      </DialogRoot>
    </Fragment>
  );
}

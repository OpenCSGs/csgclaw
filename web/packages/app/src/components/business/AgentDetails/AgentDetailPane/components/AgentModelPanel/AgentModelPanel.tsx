import type { ReactNode } from "react";
import profileFormLayoutStyles from "@/components/business/ProfileControls/ProfileFormLayout.module.css";
import agentDetailPaneStyles from "@/components/business/AgentDetails/AgentDetailPane/AgentDetailPane.module.css";
import { AgentModelConfiguration } from "@/components/business/AgentModelConfiguration";
import { AlertCircle, RefreshCw } from "lucide-react";
import { errorMessage } from "@/api/client";
import { Button } from "@/components/ui";
import type { AgentDraft } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import { type modelProviderSelectOptionsFromCatalog } from "@/models/modelProviders";
import { localizeAPIError } from "@/shared/i18n";

type ModelProviderSelectOption = ReturnType<typeof modelProviderSelectOptionsFromCatalog>[number];
type UpdateAgentDraft = (patch: Partial<AgentDraft>) => void;

type AgentModelPanelProps = {
  draft: AgentDraft;
  modelBusy: boolean;
  modelError: unknown;
  modelDirectory?: readonly string[] | null;
  modelSaveError?: string;
  modelSaveBlocked: boolean;
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
  modelDirectory,
  modelSaveError,
  modelSaveBlocked,
  onRetryModels,
  onRequireOpenCSGAuth,
  providerOptions,
  selectedModelValue,
  selectedProviderID,
  selectedProviderModels,
  t,
  updateDraft,
}: AgentModelPanelProps) {
  const modelOutsideDirectory = Boolean(
    selectedModelValue && modelDirectory && !selectedProviderModels.includes(selectedModelValue),
  );
  const missingModelMessage = modelSaveBlocked
    ? `${t("profileModelOutsideDirectory")} ${t("profileModelSelectAvailableToSave")}`
    : t("profileModelOutsideDirectory");
  let modelHint: ReactNode;
  if (selectedModelValue && !modelSaveError) {
    if (modelOutsideDirectory) {
      modelHint = <span className="text-[var(--csgclaw-ui-danger)]">{missingModelMessage}</span>;
    } else if (modelDirectory === null || modelBusy) {
      modelHint = t("profileModelAvailabilityUnknown");
    }
  }
  return (
    <section
      id="agent-profile-model"
      className={`profile-section ${profileFormLayoutStyles["profile-section"]} ${agentDetailPaneStyles["agent-profile-scroll-target"]}`}
    >
      <div className="profile-section-heading">
        <div className={`profile-section-title ${profileFormLayoutStyles["profile-section-title"]}`}>
          {t("profileModelSection")}
        </div>
        <p className="profile-section-description">{t("profileModelSectionDescription")}</p>
      </div>
      <div className="agent-section-form">
        <div className="agent-page-form-content agent-model-form-content">
          {selectedProviderID && modelDirectory?.length === 0 && onRetryModels ? (
            <div className="flex flex-wrap items-center gap-2">
              <span role="status">{t("modelProviderNoModels")}</span>
              <Button
                loading={modelBusy}
                loadingLabel={t("profileLoadingModels")}
                size="sm"
                variant="secondaryGray"
                onClick={() => void onRetryModels()}
              >
                <RefreshCw aria-hidden="true" size={15} strokeWidth={2} />
                {t("profileModelRefreshDirectory")}
              </Button>
            </div>
          ) : null}
          <AgentModelConfiguration
            draft={draft}
            providerOptions={providerOptions}
            providerID={selectedProviderID}
            modelID={selectedModelValue}
            models={selectedProviderModels}
            busy={modelBusy}
            deferModelSelection={modelDirectory !== undefined}
            modelHint={modelHint}
            modelSaveError={modelOutsideDirectory && modelSaveError ? missingModelMessage : modelSaveError}
            onPatch={updateDraft}
            onRequireOpenCSGAuth={onRequireOpenCSGAuth}
            t={t}
          />
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
    </section>
  );
}

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
  onRetryModels,
  onRequireOpenCSGAuth,
  providerOptions,
  selectedModelValue,
  selectedProviderID,
  selectedProviderModels,
  t,
  updateDraft,
}: AgentModelPanelProps) {
  return (
    <section id="agent-profile-model" className="profile-section agent-profile-scroll-target">
      <div className="profile-section-heading">
        <div className="profile-section-title">{t("profileModelSection")}</div>
        <p className="profile-section-description">{t("profileModelSectionDescription")}</p>
      </div>
      <div className="agent-section-form">
        <div className="agent-page-form-content agent-model-form-content">
          <AgentModelConfiguration
            draft={draft}
            providerOptions={providerOptions}
            providerID={selectedProviderID}
            modelID={selectedModelValue}
            models={selectedProviderModels}
            busy={modelBusy}
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

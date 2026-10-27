import { AgentModelConfiguration } from "@/components/business/AgentModelConfiguration";
import { BOT_TYPE_NORMAL } from "@/shared/constants/agents";
import { useEffect, useState, type SetStateAction } from "react";
import {
  AgentCreateProgress,
  type AgentCreateProgressProps,
  EnvKeyValueEditor,
  isBlank,
  MCPServersPanel,
  NotifierControls,
  requiredFieldLabel,
  RuntimeOptionsFields,
} from "@/components/business/ProfileControls";
import { Button, Select } from "@/components/ui";
import { AgentAvatarContent, AgentAvatarPicker } from "@/components/business/AgentAvatar";
import { RuntimeInstallProgress } from "@/components/business/RuntimeInstallProgress";
import type { AgentRuntimeInstallation } from "@/api/agentRuntimes";
import {
  agentRuntimeKind,
  agentDraftMissingRequiredEnv,
  agentTemplateDisplayName,
  composeLegacyRuntimeKind,
  formatRuntimeKindLabel,
  isNotificationBotDraftContext,
  normalizeRuntimeName,
  notifierFormIsComplete,
  runtimeOptionSchemasForAgent,
  supportsMCPServers,
} from "@/models/agents";
import type { AgentDraft, AgentLike, RuntimeBootstrapConfig } from "@/models/agents";
import {
  modelIDsForProviderSelection,
  providerNameForProviderID,
  modelProviderAvatarPath,
  modelProviderSelectOptionsFromCatalog,
  selectorForProviderModel,
  type ModelProviderCatalog,
  type ModelProviderOption,
} from "@/models/modelProviders";
import type { TranslateFn } from "@/models/conversations";
import type { LocaleCode } from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { AgentCreationResources } from "@/models/agentCreation";
import { AgentCreateWizard } from "./AgentCreateWizard";
import { ModalCloseButton } from "./ModalCloseButton";
import styles from "./AgentProfileModal.module.css";

type AgentModalMode = "create" | "edit";
type AgentCreateMode = "template" | "custom";
type AgentDraftUpdate = SetStateAction<AgentDraft | null>;
type VoidOrPromise = void | Promise<void>;

export type AgentProfileModalProps = {
  agentBusy?: boolean;
  agentInitializing?: boolean;
  agentCreateBotKind: string;
  agentCreateMode?: AgentCreateMode;
  creationWizard?: boolean;
  openCSGAuthenticated?: boolean;
  agentDraft: AgentDraft;
  agentError?: string;
  agentBillingURL?: string;
  agentModelBusy?: boolean;
  agentModalMode: AgentModalMode;
  agentModelOptions?: ModelProviderOption[];
  modelProviders?: ModelProviderCatalog | null;
  agentModels?: string[];
  agentProgress?: AgentCreateProgressProps["progress"];
  runtimeInstallBusy?: boolean;
  runtimeInstallError?: string;
  runtimeInstallation?: AgentRuntimeInstallation | null;
  authBusyProvider?: string;
  authStatuses?: unknown;
  bootstrapConfig?: RuntimeBootstrapConfig | null;
  editingAgent?: AgentLike | null;
  hubTemplates?: HubTemplate[];
  managerAgent?: AgentLike | null;
  notifierWebhookPublicOrigin?: string;
  onAgentCreateBotKindChange: (kind: string) => void;
  onBrowseTemplates?: () => void;
  onAgentDraftChange: (update: AgentDraftUpdate) => void;
  onAgentModelsReset: () => void;
  onClose: () => void;
  onProviderLogin?: (provider: string) => VoidOrPromise;
  onRequireOpenCSGAuth?: () => boolean;
  onInstallRuntime?: (name: string) => VoidOrPromise;
  onSave: (resources?: AgentCreationResources) => VoidOrPromise;
  locale: LocaleCode;
  t: TranslateFn;
};

export function AgentProfileModal({
  t,
  agentModalMode,
  agentCreateBotKind,
  agentCreateMode = "custom",
  creationWizard = false,
  openCSGAuthenticated = false,
  editingAgent,
  agentDraft,
  onBrowseTemplates,
  onAgentDraftChange,
  hubTemplates = [],
  bootstrapConfig = null,
  agentModelOptions = [],
  modelProviders = null,
  agentModels = [],
  agentModelBusy = false,
  notifierWebhookPublicOrigin = "",
  agentError = "",
  agentBillingURL = "",
  agentProgress = null,
  agentBusy = false,
  agentInitializing = false,
  runtimeInstallBusy = false,
  runtimeInstallError = "",
  runtimeInstallation = null,
  locale,
  onClose,
  onRequireOpenCSGAuth = () => true,
  onInstallRuntime = () => {},
  onSave,
}: AgentProfileModalProps) {
  const [mcpServersInvalid, setMCPServersInvalid] = useState(false);
  const createBotKind = agentModalMode === "create" ? agentCreateBotKind : undefined;
  const isNotificationContext = isNotificationBotDraftContext(agentDraft, editingAgent, createBotKind);
  const isWorkerCreate = agentModalMode === "create" && !isNotificationContext;
  const isTemplateCreate = isWorkerCreate && agentCreateMode === "template";
  const missingRequiredEnv = isTemplateCreate && agentDraftMissingRequiredEnv(agentDraft);
  const isCustomCreate = isWorkerCreate && agentCreateMode === "custom";
  const useWizard = creationWizard && isCustomCreate;
  const showMCPServers = !useWizard && !isNotificationContext && supportsMCPServers(agentDraft.runtime_kind);
  const runtimeOptionSchemas = isNotificationContext
    ? []
    : runtimeOptionSchemasForAgent(
        agentDraft.runtime_kind,
        agentModalMode === "edit" ? editingAgent : null,
        bootstrapConfig,
      );
  const fallbackProviderID = String(agentDraft.model_provider_id || "").trim();
  const fallbackModelOptions =
    agentModelOptions.length > 0
      ? agentModelOptions
      : fallbackProviderID
        ? agentModels.map((model) => ({
            value: selectorForProviderModel(fallbackProviderID, model),
            label: `${fallbackProviderID} / ${model}`,
            providerID: fallbackProviderID,
            providerDisplayName: fallbackProviderID,
            providerAvatar: modelProviderAvatarPath(fallbackProviderID),
            modelID: model,
          }))
        : [];
  const providerOptions = modelProviderSelectOptionsFromCatalog(modelProviders, fallbackModelOptions);
  const selectedProviderID =
    agentDraft.model_provider_id ||
    providerOptions.find((option) => option.models.includes(agentDraft.model_id))?.id ||
    "";
  useEffect(() => {
    if (String(agentDraft.model_provider_id || "").trim() || !selectedProviderID) {
      return;
    }
    onAgentDraftChange({
      ...agentDraft,
      provider: providerNameForProviderID(selectedProviderID),
      model_provider_id: selectedProviderID,
    });
  }, [agentDraft, onAgentDraftChange, selectedProviderID]);
  const selectedProvider = providerOptions.find((option) => option.id === selectedProviderID) ?? null;
  const selectedProviderModels = modelIDsForProviderSelection(selectedProviderID, selectedProvider?.models ?? []);
  const selectedModelValue = agentDraft.model_id || "";
  const selectedWorkerTemplate = hubTemplates.find((item) => item.id === agentDraft.from_template) ?? null;
  const selectedTemplateName =
    agentTemplateDisplayName(selectedWorkerTemplate) || agentDraft.template_name || agentDraft.from_template;
  const sandboxEnabled = false;
  const runtimeChoices = Array.isArray(bootstrapConfig?.worker_runtime_choices)
    ? bootstrapConfig.worker_runtime_choices
    : [];
  const codexChoice = runtimeChoices.find(
    (item) => !item?.sandbox_enabled && normalizeRuntimeName(item?.name) === "codex",
  );
  const hostRuntimeChoices = runtimeChoices.filter((item) => !item?.sandbox_enabled);
  const requestedRuntimeName = normalizeRuntimeName(agentDraft.runtime_name || "codex");
  const selectedRuntimeName =
    !sandboxEnabled &&
    hostRuntimeChoices.length > 0 &&
    !hostRuntimeChoices.some((item) => normalizeRuntimeName(item?.name) === requestedRuntimeName)
      ? normalizeRuntimeName(codexChoice?.name || hostRuntimeChoices[0]?.name || "codex")
      : requestedRuntimeName;
  const selectedRuntimeChoice = runtimeChoices.find((item) => {
    const choiceSandboxEnabled = Boolean(item?.sandbox_enabled);
    const choiceRuntimeName = normalizeRuntimeName(item?.name);
    return choiceSandboxEnabled === sandboxEnabled && choiceRuntimeName === selectedRuntimeName;
  });
  const selectedRuntimeMissing = isWorkerCreate && selectedRuntimeChoice?.installed === false;
  const selectedRuntimeUnavailable = selectedRuntimeMissing;
  const selectedRuntimeInstallable = selectedRuntimeMissing && selectedRuntimeChoice?.installable === true;
  const runtimeMessageKey = String(selectedRuntimeChoice?.message_code || "").trim();
  const localizedRuntimeMessage = runtimeMessageKey ? t(`errors.${runtimeMessageKey}`) : "";
  const resolvedRuntimeMessage =
    (runtimeMessageKey && localizedRuntimeMessage !== `errors.${runtimeMessageKey}` ? localizedRuntimeMessage : "") ||
    selectedRuntimeChoice?.message ||
    t("runtimeSandboxUnavailableReason");
  const selectedRuntimeUnavailableMessage =
    selectedRuntimeMissing && !selectedRuntimeInstallable
      ? sandboxEnabled
        ? t("runtimeSandboxUnavailable", {
            reason: resolvedRuntimeMessage,
          })
        : resolvedRuntimeMessage || t("runtimeCodexNotInstalled")
      : "";
  const runtimeAvailabilityNotice = selectedRuntimeInstallable ? (
    <div className="agent-runtime-install-notice" role="status" aria-live="polite">
      <div className="agent-runtime-install-copy">
        <strong>{t("runtimeInstallRequiredTitle")}</strong>
        <span>{t("runtimeInstallRequiredDescription")}</span>
        {runtimeInstallError ? <small className="field-warning">{runtimeInstallError}</small> : null}
      </div>
      <Button
        variant="primary"
        size="sm"
        loading={runtimeInstallBusy}
        loadingLabel={t("computerRuntimeInstalling")}
        onClick={() => void onInstallRuntime(selectedRuntimeName)}
      >
        {t("computerRuntimeInstall")}
      </Button>
      {runtimeInstallBusy ? (
        <div className="agent-runtime-install-progress">
          <RuntimeInstallProgress installation={runtimeInstallation} t={t} />
        </div>
      ) : null}
    </div>
  ) : selectedRuntimeUnavailableMessage ? (
    <div className="agent-runtime-install-notice is-error" role="alert">
      <small className="field-warning">{selectedRuntimeUnavailableMessage}</small>
    </div>
  ) : null;

  useEffect(() => {
    if (!showMCPServers) {
      setMCPServersInvalid(false);
    }
  }, [showMCPServers]);

  const saveDisabled =
    agentBusy ||
    agentInitializing ||
    mcpServersInvalid ||
    isBlank(agentDraft.name) ||
    (isNotificationContext
      ? !notifierFormIsComplete(agentDraft, editingAgent)
      : isTemplateCreate
        ? !agentDraft.from_template ||
          !agentDraft.model_provider_id ||
          !agentDraft.model_id ||
          missingRequiredEnv ||
          selectedRuntimeUnavailable
        : !agentDraft.model_provider_id || !agentDraft.model_id || selectedRuntimeUnavailable);

  const editor = (
    <div className={useWizard ? styles.wizardProfile : "profile-editor-shell"}>
      {agentInitializing ? <p role="status">{t("agentWizardLoadingConfiguration")}</p> : null}
      <section className="profile-section agent-identity-section">
        {!isNotificationContext ? (
          <div className="profile-section-heading">
            <div className="profile-section-title">{t("profileBasics")}</div>
            <p className="profile-section-description">{t("profileBasicsDescription")}</p>
          </div>
        ) : null}
        <div className="agent-section-form">
          <div className="agent-section-form-content agent-basics-form-content">
            <div className="agent-identity-layout">
              <div className="field agent-avatar-field">
                <span className="field-label">{t("agentAvatar")}</span>
                <AgentAvatarPicker
                  value={agentDraft.avatar}
                  t={t}
                  mode="edit"
                  onChange={(avatar) => onAgentDraftChange({ ...agentDraft, avatar })}
                />
              </div>
              <label className="field agent-name-field">
                {requiredFieldLabel(t("agentName"))}
                <input
                  value={agentDraft.name}
                  required
                  aria-required="true"
                  onInput={(event) => onAgentDraftChange({ ...agentDraft, name: event.currentTarget.value })}
                  placeholder={t("agentNamePlaceholder")}
                />
              </label>
              <label className="field agent-description-field">
                <span>{t("agentDescription")}</span>
                <textarea
                  className="compact-textarea"
                  value={agentDraft.description}
                  onInput={(event) => onAgentDraftChange({ ...agentDraft, description: event.currentTarget.value })}
                />
              </label>
            </div>
            {isCustomCreate && onBrowseTemplates ? (
              <div className={styles.browseTemplates}>
                <span>{t("iaCreateAgentBrowseTemplatesHint")}</span>
                <Button variant="secondaryGray" size="sm" onClick={onBrowseTemplates} disabled={agentBusy}>
                  {t("iaCreateAgentBrowseTemplates")}
                </Button>
              </div>
            ) : null}
          </div>
        </div>
      </section>
      {isTemplateCreate ? (
        <section className="profile-section">
          <div className="profile-section-heading">
            <div className="profile-section-title">{t("createAgentTemplateSectionTitle")}</div>
            <p className="profile-section-description">{t("createAgentTemplateSectionDescription")}</p>
          </div>
          <div className="agent-section-form">
            <div className="profile-grid profile-grid-compact agent-basics-grid">
              <div className="field span-2">
                <span>{t("templateLabel")}</span>
                <div className={styles.templateCard}>
                  <span className={styles.templateAvatar} aria-hidden="true">
                    <AgentAvatarContent
                      avatar={selectedWorkerTemplate?.avatar}
                      fallback={selectedTemplateName?.slice(0, 1)}
                    />
                  </span>
                  <span className={styles.templateCopy}>
                    <strong>{selectedTemplateName}</strong>
                    <small>{selectedWorkerTemplate?.description}</small>
                  </span>
                  {onBrowseTemplates ? (
                    <Button variant="secondaryGray" size="sm" onClick={onBrowseTemplates} disabled={agentBusy}>
                      {t("iaCreateAgentSwitchTemplate")}
                    </Button>
                  ) : null}
                </div>
                <small className="field-hint">{t("templateHelp")}</small>
              </div>
              {runtimeAvailabilityNotice}
              {selectedWorkerTemplate?.image_env?.length ? (
                <div className="field span-2">
                  <span>{t("profileEnv")}</span>
                  <EnvKeyValueEditor
                    rows={agentDraft.envRows}
                    t={t}
                    onChange={(rows) => onAgentDraftChange({ ...agentDraft, envRows: rows })}
                  />
                </div>
              ) : null}
            </div>
          </div>
        </section>
      ) : null}
      {!isNotificationContext && !isTemplateCreate ? (
        <section className="profile-section">
          <div className="profile-section-heading">
            <div className="profile-section-title">{t("profileRuntimeSection")}</div>
            <p className="profile-section-description">{t("profileRuntimeSectionDescription")}</p>
          </div>
          <div className="agent-section-form">
            <div
              className={useWizard ? styles.wizardRuntimeGrid : "profile-grid profile-grid-compact agent-basics-grid"}
            >
              {isWorkerCreate ? (
                <div className={useWizard ? styles.wizardRuntimeSelector : "agent-runtime-image-row"}>
                  <label className="field">
                    <span>{t("profileRuntimeKind")}</span>
                    <Select
                      value={selectedRuntimeName || "codex"}
                      onValueChange={(value) => {
                        onAgentDraftChange({
                          ...agentDraft,
                          bot_type: BOT_TYPE_NORMAL,
                          sandbox_enabled: false,
                          runtime_name: normalizeRuntimeName(value) || "codex",
                          runtime_kind: composeLegacyRuntimeKind(value, false) || "codex",
                          image: "",
                          from_template: "",
                          template_name: "",
                        });
                      }}
                      triggerProps={{ "aria-label": t("profileRuntimeKind") }}
                      options={hostRuntimeChoices.map((option) => ({
                        value: normalizeRuntimeName(option.name) || "",
                        label: option.label || normalizeRuntimeName(option.name) || "",
                        disabled: option.installed === false && option.installable !== true,
                      }))}
                    />
                  </label>
                  {runtimeAvailabilityNotice}
                </div>
              ) : agentModalMode === "edit" ? (
                <label className="field span-2">
                  <span>{t("profileRuntimeKind")}</span>
                  <input value={formatRuntimeKindLabel(agentRuntimeKind(editingAgent), t)} readOnly disabled />
                </label>
              ) : null}
              {runtimeOptionSchemas.length > 0 ? (
                <RuntimeOptionsFields
                  draft={agentDraft}
                  locale={locale}
                  schemas={runtimeOptionSchemas}
                  onDraftChange={onAgentDraftChange}
                  directoryPickerAvailable={bootstrapConfig?.directory_picker_available !== false}
                  embedded
                />
              ) : null}
              {showMCPServers ? (
                <MCPServersPanel
                  draft={agentDraft}
                  t={t}
                  onDraftChange={onAgentDraftChange}
                  onInvalidChange={setMCPServersInvalid}
                />
              ) : null}
            </div>
          </div>
        </section>
      ) : null}
      {isNotificationContext ? (
        <NotifierControls
          agentID={agentModalMode === "edit" ? editingAgent?.id || "" : ""}
          draft={agentDraft}
          hideHeading
          t={t}
          webhookPublicOrigin={notifierWebhookPublicOrigin}
          onPatch={(patch) => onAgentDraftChange({ ...agentDraft, ...patch })}
        />
      ) : (
        <section className="profile-section">
          <div className="profile-section-heading">
            <div className="profile-section-title">{t("profileModelSection")}</div>
            <p className="profile-section-description">{t("profileModelSectionDescription")}</p>
          </div>
          <div className="agent-section-form">
            <div className="agent-section-form-content agent-model-form-content">
              <AgentModelConfiguration
                draft={agentDraft}
                providerOptions={providerOptions}
                providerID={selectedProviderID}
                modelID={selectedModelValue}
                models={selectedProviderModels}
                busy={agentModelBusy || agentBusy}
                onPatch={(patch) => onAgentDraftChange({ ...agentDraft, ...patch })}
                onRequireOpenCSGAuth={onRequireOpenCSGAuth}
                t={t}
              />
            </div>
          </div>
        </section>
      )}
      {!isNotificationContext && !isTemplateCreate ? (
        <>
          {!useWizard ? (
            <section className="profile-section agent-instructions-section">
              <div className="profile-grid-compact">
                <label className="field span-2">
                  <span>{t("agentInstructions")}</span>
                  <textarea
                    className="compact-textarea"
                    value={agentDraft.instructions || ""}
                    placeholder={t("agentInstructionsPlaceholder")}
                    onInput={(event) => onAgentDraftChange({ ...agentDraft, instructions: event.currentTarget.value })}
                  />
                </label>
              </div>
            </section>
          ) : null}
          <details className="profile-section agent-advanced-section">
            <summary className="profile-section-title agent-advanced-summary">{t("profileAdvanced")}</summary>
            <div className="profile-advanced-grid">
              <div className="field">
                <span>{t("profileEnv")}</span>
                <EnvKeyValueEditor
                  rows={agentDraft.envRows}
                  t={t}
                  onChange={(rows) => onAgentDraftChange({ ...agentDraft, envRows: rows })}
                />
              </div>
            </div>
          </details>
        </>
      ) : null}
    </div>
  );
  if (useWizard)
    return (
      <AgentCreateWizard
        profile={editor}
        instructions={agentDraft.instructions || ""}
        onInstructionsChange={(instructions) => onAgentDraftChange({ ...agentDraft, instructions })}
        valid={!saveDisabled}
        busy={agentBusy}
        error={
          agentError ? (
            <>
              {agentError}
              {agentBillingURL ? (
                <a href={agentBillingURL} target="_blank" rel="noopener noreferrer">
                  {t("rechargeAccount")}
                </a>
              ) : null}
            </>
          ) : undefined
        }
        progress={agentProgress}
        authenticated={openCSGAuthenticated}
        onRequireAuth={onRequireOpenCSGAuth}
        onClose={onClose}
        onSave={onSave}
        t={t}
      />
    );

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-card profile-modal agent-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <div className="modal-title">
              {agentModalMode === "create" ? t("createAgentTitle") : t("editAgentTitle")}
            </div>
            <div className="modal-subtitle">
              {agentModalMode === "create"
                ? isNotificationContext
                  ? t("createAgentSubtitleNotifier")
                  : t("createAgentSubtitle")
                : t("editAgentSubtitle")}
            </div>
          </div>
          <ModalCloseButton label={t("close")} onClose={onClose} />
        </div>
        {editor}
        {agentError ? (
          <div className="form-error">
            {agentError}
            {agentBillingURL ? (
              <>
                {" "}
                <a className="agent-billing-action" href={agentBillingURL} target="_blank" rel="noopener noreferrer">
                  {t("rechargeAccount")}
                </a>
              </>
            ) : null}
          </div>
        ) : null}
        <AgentCreateProgress progress={agentProgress} t={t} />
        <div className="modal-actions">
          <Button variant="secondaryGray" size="md" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" size="md" disabled={saveDisabled} loading={agentBusy} onClick={() => void onSave()}>
            {agentModalMode === "create" ? t("agentCreateSave") : t("agentUpdateSave")}
          </Button>
        </div>
      </div>
    </div>
  );
}

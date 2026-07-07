import {
  BOT_TYPE_NORMAL,
  DEFAULT_RUNTIME_KIND,
} from "@/shared/constants/agents";
import { useEffect, useRef, useState, type SetStateAction } from "react";
import {
  AgentCreateProgress,
  type AgentCreateProgressProps,
  EnvKeyValueEditor,
  FieldHelpTooltip,
  isBlank,
  ModelOptionLabel,
  NotifierControls,
  requiredFieldLabel,
  RuntimeOptionsFields,
} from "@/components/business/ProfileControls";
import { Button, Select } from "@/components/ui";
import { AgentAvatarPicker } from "@/components/business/AgentAvatar";
import {
  agentRuntimeKind,
  agentDraftMissingRequiredEnv,
  agentCreateTemplateLocked,
  applyTemplateToDraft,
  composeLegacyRuntimeKind,
  formatRuntimeKindLabel,
  isNotificationBotDraftContext,
  normalizeRuntimeKind,
  normalizeRuntimeName,
  normalizeTemplateSelection,
  notifierFormIsComplete,
  pickDefaultAgentTemplate,
  defaultWorkerImageForRuntime,
  runtimeOptionSchemasForAgent,
  templateMatchesRuntime,
  workerSelectableTemplates,
} from "@/models/agents";
import type { AgentDraft, AgentLike, RuntimeBootstrapConfig } from "@/models/agents";
import {
  modelProviderAvatarPath,
  modelProviderSelectOptionsFromCatalog,
  providerNameForProviderID,
  selectorForProviderModel,
  type ModelProviderCatalog,
  type ModelProviderOption,
} from "@/models/modelProviders";
import type { TranslateFn } from "@/models/conversations";
import type { LocaleCode } from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import { ModalCloseButton } from "./ModalCloseButton";

type AgentModalMode = "create" | "edit";
type AgentCreateMode = "template" | "custom";
type AgentDraftUpdate = SetStateAction<AgentDraft | null>;
type VoidOrPromise = void | Promise<void>;

export type AgentProfileModalProps = {
  agentBusy?: boolean;
  agentCreateBotKind: string;
  agentCreateMode?: AgentCreateMode;
  agentDraft: AgentDraft;
  agentError?: string;
  agentModelBusy?: boolean;
  agentModalMode: AgentModalMode;
  agentModelOptions?: ModelProviderOption[];
  modelProviders?: ModelProviderCatalog | null;
  agentModels?: string[];
  agentProgress?: AgentCreateProgressProps["progress"];
  authBusyProvider?: string;
  authStatuses?: unknown;
  bootstrapConfig?: RuntimeBootstrapConfig | null;
  editingAgent?: AgentLike | null;
  hubTemplates?: HubTemplate[];
  managerAgent?: AgentLike | null;
  notifierWebhookPublicOrigin?: string;
  onAgentCreateBotKindChange: (kind: string) => void;
  onAgentCreateModeChange?: (mode: AgentCreateMode) => void;
  onAgentDraftChange: (update: AgentDraftUpdate) => void;
  onAgentModelsReset: () => void;
  onClose: () => void;
  onProviderLogin?: (provider: string) => VoidOrPromise;
  onSave: () => VoidOrPromise;
  locale: LocaleCode;
  t: TranslateFn;
};

export function AgentProfileModal({
  t,
  agentModalMode,
  agentCreateBotKind,
  agentCreateMode = "template",
  editingAgent,
  agentDraft,
  onAgentCreateModeChange = () => {},
  onAgentDraftChange,
  onAgentModelsReset,
  hubTemplates = [],
  bootstrapConfig = null,
  managerAgent = null,
  agentModelOptions = [],
  modelProviders = null,
  agentModels = [],
  agentModelBusy = false,
  notifierWebhookPublicOrigin = "",
  agentError = "",
  agentProgress = null,
  agentBusy = false,
  locale,
  onClose,
  onSave,
}: AgentProfileModalProps) {
  const [isEditorScrolling, setIsEditorScrolling] = useState(false);
  const editorScrollTimerRef = useRef<number | null>(null);
  const lastTemplateIDRef = useRef("");
  const createBotKind = agentModalMode === "create" ? agentCreateBotKind : undefined;
  const isNotificationContext = isNotificationBotDraftContext(agentDraft, editingAgent, createBotKind);
  const isWorkerCreate = agentModalMode === "create" && !isNotificationContext;
  const isTemplateCreate = isWorkerCreate && agentCreateMode === "template";
  const missingRequiredEnv = isTemplateCreate && agentDraftMissingRequiredEnv(agentDraft);
  const isCustomCreate = isWorkerCreate && agentCreateMode === "custom";
  const templateLocked = agentCreateTemplateLocked(agentDraft, agentModalMode);
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
  const selectedProvider = providerOptions.find((option) => option.id === selectedProviderID) ?? null;
  const selectedProviderModels = selectedProvider?.models ?? [];
  const selectedModelValue = agentDraft.model_id || "";
  const workerTemplates = workerSelectableTemplates(hubTemplates);
  const selectedWorkerTemplate = workerTemplates.find((item) => item.id === agentDraft.from_template) ?? null;
  const sandboxEnabled = Boolean(agentDraft.sandbox_enabled);
  const runtimeChoices = Array.isArray(bootstrapConfig?.worker_runtime_choices)
    ? bootstrapConfig.worker_runtime_choices
    : [];
  const codexChoice = runtimeChoices.find(
    (item) => !item?.sandbox_enabled && normalizeRuntimeName(item?.name) === "codex",
  );
  const sandboxRuntimeChoices = runtimeChoices.filter((item) => item?.sandbox_enabled);
  const defaultSandboxRuntimeName =
    normalizeRuntimeName(sandboxRuntimeChoices.find((item) => normalizeRuntimeName(item?.name) === "picoclaw")?.name) ||
    normalizeRuntimeName(sandboxRuntimeChoices[0]?.name || "picoclaw");
  const selectedRuntimeName = normalizeRuntimeName(
    agentDraft.runtime_name || (sandboxEnabled ? defaultSandboxRuntimeName : "codex"),
  );

  function defaultCustomWorkerDraft(baseDraft: AgentDraft): AgentDraft {
    const codexAvailable = codexChoice?.installed !== false;
    const runtimeName = codexAvailable ? "codex" : defaultSandboxRuntimeName;
    const nextSandboxEnabled = !codexAvailable;
    const runtimeKind = composeLegacyRuntimeKind(runtimeName, nextSandboxEnabled) || DEFAULT_RUNTIME_KIND;
    return {
      ...baseDraft,
      bot_type: BOT_TYPE_NORMAL,
      description: "",
      envRows: [],
      name: "",
      sandbox_enabled: nextSandboxEnabled,
      runtime_name: runtimeName,
      runtime_kind: runtimeKind,
      image: nextSandboxEnabled
        ? defaultWorkerImageForRuntime(
            hubTemplates,
            runtimeKind,
            bootstrapConfig,
            baseDraft.default_image || managerAgent?.image || "",
          )
        : "",
      from_template: "",
      template_name: "",
    };
  }

  useEffect(() => {
    if (isTemplateCreate) {
      const nextTemplateID = String(agentDraft.from_template || "").trim();
      if (nextTemplateID) {
        lastTemplateIDRef.current = nextTemplateID;
      }
    }
  }, [agentDraft.from_template, isTemplateCreate]);

  function setSandboxEnabled(nextSandboxEnabled: boolean) {
    if (!isWorkerCreate) {
      return;
    }
    if (nextSandboxEnabled) {
      const runtimeName = defaultSandboxRuntimeName;
      const runtimeKind = composeLegacyRuntimeKind(runtimeName, true) || DEFAULT_RUNTIME_KIND;
      onAgentDraftChange({
        ...agentDraft,
        bot_type: BOT_TYPE_NORMAL,
        sandbox_enabled: true,
        runtime_name: runtimeName,
        runtime_kind: runtimeKind,
        image: defaultWorkerImageForRuntime(
          hubTemplates,
          runtimeKind,
          bootstrapConfig,
          agentDraft.default_image || managerAgent?.image || "",
        ),
        from_template: "",
        template_name: "",
      });
      onAgentModelsReset();
      return;
    }
    onAgentDraftChange({
      ...agentDraft,
      bot_type: BOT_TYPE_NORMAL,
      sandbox_enabled: false,
      runtime_name: "codex",
      runtime_kind: "codex",
      image: "",
      from_template: "",
      template_name: "",
    });
    onAgentModelsReset();
  }

  useEffect(
    () => () => {
      if (editorScrollTimerRef.current) {
        window.clearTimeout(editorScrollTimerRef.current);
      }
    },
    [],
  );

  function onEditorShellScroll() {
    setIsEditorScrolling(true);
    if (editorScrollTimerRef.current) {
      window.clearTimeout(editorScrollTimerRef.current);
    }
    editorScrollTimerRef.current = window.setTimeout(() => {
      setIsEditorScrolling(false);
      editorScrollTimerRef.current = null;
    }, 700);
  }

  function switchCreateMode(nextMode: AgentCreateMode) {
    if (!isWorkerCreate || nextMode === agentCreateMode) {
      return;
    }
    onAgentCreateModeChange(nextMode);
    if (nextMode === "template") {
      const nextTemplate = normalizeTemplateSelection(
        hubTemplates.find((item) => item.id === lastTemplateIDRef.current) ||
          hubTemplates.find((item) => item.id === agentDraft.from_template) ||
          pickDefaultAgentTemplate(hubTemplates, agentDraft.runtime_kind, bootstrapConfig) ||
          null,
      );
      onAgentDraftChange((current) =>
        current ? applyTemplateToDraft(current, nextTemplate, bootstrapConfig, managerAgent?.image || "") : current,
      );
      return;
    }
    onAgentDraftChange((current) => (current ? defaultCustomWorkerDraft(current) : current));
  }

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
        <div
          className={`profile-editor-shell${isEditorScrolling ? " is-scrolling" : ""}`}
          onScroll={onEditorShellScroll}
        >
          <section className="profile-section agent-identity-section">
            {!isNotificationContext ? (
              <div className="profile-section-heading">
                <div className="profile-section-title">
                  {t("profileBasics")}
                </div>
                <p className="profile-section-description">
                  {t("profileBasicsDescription")}
                </p>
              </div>
            ) : null}
            <div className="agent-section-form">
              <div className="agent-section-form-content agent-basics-form-content">
                {isWorkerCreate ? (
                  <div
                    className="workspace-tabbar agent-create-kind-tabbar"
                    role="tablist"
                    aria-label={t("createAgentModeTabAriaLabel")}
                  >
                    <Button
                      className="workspace-tab"
                      active={isTemplateCreate}
                      role="tab"
                      aria-selected={isTemplateCreate}
                      onClick={() => switchCreateMode("template")}
                    >
                      <span className="workspace-tab-copy">
                        <strong>{t("createAgentModeTemplate")}</strong>
                        <small>{t("createAgentModeTemplateDescription")}</small>
                      </span>
                    </Button>
                    <Button
                      className="workspace-tab"
                      active={isCustomCreate}
                      role="tab"
                      aria-selected={isCustomCreate}
                      onClick={() => switchCreateMode("custom")}
                    >
                      <span className="workspace-tab-copy">
                        <strong>{t("createAgentModeCustom")}</strong>
                        <small>{t("createAgentModeCustomDescription")}</small>
                      </span>
                    </Button>
                  </div>
                ) : null}
                {!isTemplateCreate ? (
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
                  <label className="field span-2">
                    <span>{t("templateLabel")}</span>
                    <Select
                      value={agentDraft.from_template || ""}
                      onValueChange={(value) => {
                        const nextTemplate = normalizeTemplateSelection(
                          hubTemplates.find((item) => item.id === value) || null,
                        );
                        onAgentDraftChange((current) =>
                          current
                            ? applyTemplateToDraft(current, nextTemplate, bootstrapConfig, managerAgent?.image || "")
                            : current,
                        );
                      }}
                      triggerProps={{ "aria-label": t("templateLabel") }}
                      options={workerTemplates.filter((item) => item.id).map((item) => ({
                        value: item.id || "",
                        label: item.name || item.id || "",
                        description: String(item.description || "").trim() || undefined,
                      }))}
                    />
                    <small className="field-hint">{t("templateHelp")}</small>
                  </label>
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
                <div className="profile-grid profile-grid-compact agent-basics-grid">
                  {isWorkerCreate ? (
                    <div className="field span-2 agent-fast-mode-field agent-sandbox-field">
                      <div className="field-label-with-help">
                        <span>{t("profileSandboxEnabled")}</span>
                        <FieldHelpTooltip detail={t("profileSandboxEnabledHelp")} />
                      </div>
                      <label className="selection-item compact-toggle-row agent-fast-mode-toggle agent-sandbox-toggle">
                        <input
                          type="checkbox"
                          checked={sandboxEnabled}
                          aria-label={t("profileSandboxEnabled")}
                          onChange={() => setSandboxEnabled(!sandboxEnabled)}
                        />
                        <span className="agent-sandbox-copy">
                          <strong>{sandboxEnabled ? t("statusEnabled") : t("statusDisabled")}</strong>
                        </span>
                      </label>
                    </div>
                  ) : null}
                  {isWorkerCreate ? (
                    <div className="agent-runtime-image-row">
                      <label className="field">
                        <span>{t("profileRuntimeKind")}</span>
                        {!sandboxEnabled ? (
                          <Select
                            value="codex"
                            onValueChange={(value) => {
                              onAgentDraftChange({
                                ...agentDraft,
                                bot_type: BOT_TYPE_NORMAL,
                                sandbox_enabled: false,
                                runtime_name: normalizeRuntimeName(value) || "codex",
                                runtime_kind: "codex",
                                image: "",
                                from_template: "",
                                template_name: "",
                              });
                            }}
                            triggerProps={{ "aria-label": t("profileRuntimeKind") }}
                            options={[
                              {
                                value: "codex",
                                label:
                                  codexChoice?.installed === false
                                    ? t("runtimeCodexCLIUnavailable")
                                    : t("runtimeCodexCLI"),
                                disabled: codexChoice?.installed === false,
                                description: codexChoice?.message || undefined,
                              },
                            ]}
                          />
                        ) : templateLocked ? (
                          <input
                            value={formatRuntimeKindLabel(normalizeRuntimeKind(agentDraft.runtime_kind), t)}
                            readOnly
                            disabled
                          />
                        ) : (
                          <Select
                            value={
                              selectedRuntimeName || normalizeRuntimeName(sandboxRuntimeChoices[0]?.name || "picoclaw")
                            }
                            onValueChange={(value) => {
                              const runtimeName = normalizeRuntimeName(value);
                              const runtimeKind = composeLegacyRuntimeKind(runtimeName, true) || DEFAULT_RUNTIME_KIND;
                              const currentTemplate = normalizeTemplateSelection(
                                hubTemplates.find((item) => item.id === agentDraft.from_template) || null,
                              );
                              const nextTemplate = templateMatchesRuntime(currentTemplate, runtimeKind)
                                ? currentTemplate
                                : pickDefaultAgentTemplate(hubTemplates, runtimeKind, bootstrapConfig);
                              let nextDraft: AgentDraft = {
                                ...agentDraft,
                                bot_type: BOT_TYPE_NORMAL,
                                role: "worker",
                                sandbox_enabled: true,
                                runtime_name: runtimeName,
                                runtime_kind: runtimeKind,
                                image: defaultWorkerImageForRuntime(
                                  hubTemplates,
                                  runtimeKind,
                                  bootstrapConfig,
                                  agentDraft.default_image || managerAgent?.image || "",
                                ),
                              };
                              nextDraft = applyTemplateToDraft(
                                nextDraft,
                                nextTemplate,
                                bootstrapConfig,
                                managerAgent?.image || "",
                              );
                              onAgentDraftChange(nextDraft);
                              onAgentModelsReset();
                            }}
                            triggerProps={{ "aria-label": t("profileRuntimeKind") }}
                            options={sandboxRuntimeChoices.map((option) => ({
                              value: normalizeRuntimeName(option.name) || "",
                              label: option.label || normalizeRuntimeName(option.name) || "",
                            }))}
                          />
                        )}
                      </label>
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
                      embedded
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
          ) : !isTemplateCreate ? (
            <>
              <section className="profile-section">
                <div className="profile-section-heading">
                  <div className="profile-section-title">{t("profileModelSection")}</div>
                  <p className="profile-section-description">{t("profileModelSectionDescription")}</p>
                </div>
                <div className="agent-section-form">
                  <div className="agent-section-form-content agent-model-form-content">
                    <div className="profile-runtime-grid agent-model-config-grid">
                      <label className="field">
                        {requiredFieldLabel(t("profileProvider"))}
                        <Select
                          value={selectedProviderID}
                          required
                          onValueChange={(value) => {
                            const nextProvider = providerOptions.find((option) => option.id === value);
                            if (!nextProvider) {
                              onAgentDraftChange({ ...agentDraft, model_id: "", model_provider_id: "" });
                              return;
                            }
                            onAgentDraftChange({
                              ...agentDraft,
                              provider: providerNameForProviderID(nextProvider.id),
                              model_provider_id: nextProvider.id,
                              model_id: nextProvider.models[0] || "",
                            });
                          }}
                          triggerProps={{ "aria-label": t("profileProvider"), "aria-required": true }}
                          options={[
                            {
                              value: "",
                              label: agentModelBusy ? t("profileLoadingModels") : t("profileProviderSelect"),
                            },
                            ...providerOptions.map((option) => ({
                              value: option.value,
                              label: (
                                <ModelOptionLabel
                                  avatar={option.avatar}
                                  model={option.displayName}
                                  provider={
                                    option.models.length
                                      ? t("modelProviderModelCount", { count: option.models.length })
                                      : t("modelProviderNoModels")
                                  }
                                />
                              ),
                              textValue: option.displayName,
                            })),
                          ]}
                        />
                      </label>
                      <label className="field">
                        {requiredFieldLabel(t("profileModel"))}
                        <Select
                          value={selectedModelValue}
                          required
                          disabled={!selectedProviderID || !selectedProviderModels.length}
                          onValueChange={(value) => onAgentDraftChange({ ...agentDraft, model_id: value })}
                          searchable
                          searchPlaceholder={t("modelProviderModelSearch")}
                          emptyLabel={t("modelProviderNoModels")}
                          triggerProps={{ "aria-label": t("profileModel"), "aria-required": true }}
                          options={[
                            {
                              value: "",
                              label: selectedProviderID ? t("profileSelectModel") : t("profileProviderSelectFirst"),
                            },
                            ...selectedProviderModels.map((modelID) => ({
                              value: modelID,
                              label: <ModelOptionLabel model={modelID} showAvatar={false} />,
                              textValue: modelID,
                            })),
                            ...(selectedModelValue && !selectedProviderModels.includes(selectedModelValue)
                              ? [
                                  {
                                    value: selectedModelValue,
                                    label: <ModelOptionLabel model={selectedModelValue} showAvatar={false} />,
                                    textValue: selectedModelValue,
                                  },
                                ]
                              : []),
                          ]}
                        />
                      </label>
                      <label className="field">
                        <span>{t("profileReasoning")}</span>
                        <Select
                          value={agentDraft.reasoning_effort}
                          onValueChange={(value) => onAgentDraftChange({ ...agentDraft, reasoning_effort: value })}
                          triggerProps={{ "aria-label": t("profileReasoning") }}
                          options={["low", "medium", "high", "xhigh"].map((effort) => ({
                            value: effort,
                            label: effort,
                          }))}
                        />
                      </label>
                      <div className="field agent-fast-mode-field">
                        <span>{t("profileFastMode")}</span>
                        <label className="selection-item compact-toggle-row agent-fast-mode-toggle">
                          <input
                            type="checkbox"
                            checked={agentDraft.enable_fast_mode}
                            aria-label={t("profileFastMode")}
                            onChange={() =>
                              onAgentDraftChange({ ...agentDraft, enable_fast_mode: !agentDraft.enable_fast_mode })
                            }
                          />
                          <small className="agent-fast-mode-help">{t("profileFastModeHelp")}</small>
                        </label>
                      </div>
                    </div>
                  </div>
                </div>
              </section>
              <section className="profile-section agent-instructions-section">
                <div className="profile-grid-compact">
                  <label className="field span-2">
                    <span>{t("agentInstructions")}</span>
                    <textarea
                      className="compact-textarea"
                      value={agentDraft.instructions || ""}
                      placeholder={t("agentInstructionsPlaceholder")}
                      onInput={(event) =>
                        onAgentDraftChange({ ...agentDraft, instructions: event.currentTarget.value })
                      }
                    />
                  </label>
                </div>
              </section>
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
        {agentError ? <div className="form-error">{agentError}</div> : null}
        <AgentCreateProgress progress={agentProgress} t={t} />
        <div className="modal-actions">
          <Button variant="secondaryGray" size="md" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            size="md"
            disabled={
              agentBusy ||
              isBlank(agentDraft.name) ||
              (isNotificationContext
                ? !notifierFormIsComplete(agentDraft, editingAgent)
                : isTemplateCreate
                  ? !agentDraft.from_template || !agentDraft.model_provider_id || !agentDraft.model_id || missingRequiredEnv
                  : !agentDraft.model_provider_id || !agentDraft.model_id)
            }
            loading={agentBusy}
            onClick={onSave}
          >
            {agentModalMode === "create" ? t("agentCreateSave") : t("agentUpdateSave")}
          </Button>
        </div>
      </div>
    </div>
  );
}

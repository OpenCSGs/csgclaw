import {
  BOT_CREATE_KIND_NOTIFICATION,
  BOT_CREATE_KIND_WORKER,
  BOT_TYPE_NORMAL,
  BOT_TYPE_NOTIFICATION,
  DEFAULT_RUNTIME_KIND,
  WORKER_RUNTIME_KIND_OPTIONS,
} from "@/shared/constants/agents";
import { useEffect, useRef, useState, type SetStateAction } from "react";
import {
  AgentCreateProgress,
  type AgentCreateProgressProps,
  EnvKeyValueEditor,
  isBlank,
  ModelOptionLabel,
  NotifierControls,
  requiredFieldLabel,
  RuntimeOptionsFields,
} from "@/components/business/ProfileControls";
import { Button, Select } from "@/components/ui";
import { AgentAvatarPicker } from "@/components/business/AgentAvatar";
import {
  agentCreateTemplateLocked,
  applyTemplateToDraft,
  ensureNotifierPullSubscriptionDraft,
  formatRuntimeKindLabel,
  isNotificationBotDraftContext,
  normalizeRuntimeKind,
  normalizeTemplateSelection,
  notifierFormIsComplete,
  pickDefaultAgentTemplate,
  runtimeOptionSchemasForAgent,
  runtimeImageForKind,
  templateMatchesRuntime,
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
type AgentDraftUpdate = SetStateAction<AgentDraft | null>;
type VoidOrPromise = void | Promise<void>;

export type AgentProfileModalProps = {
  agentBusy?: boolean;
  agentCreateBotKind: string;
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
  onAgentCreateBotKindChange,
  editingAgent,
  agentDraft,
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
  const createBotKind = agentModalMode === "create" ? agentCreateBotKind : undefined;
  const isNotificationContext = isNotificationBotDraftContext(agentDraft, editingAgent, createBotKind);
  const isWorkerCreate = agentModalMode === "create" && !isNotificationContext;
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

  function switchCreateBotKind(nextKind: string) {
    if (agentModalMode !== "create" || nextKind === agentCreateBotKind) {
      return;
    }
    onAgentCreateBotKindChange(nextKind);
    if (nextKind === BOT_CREATE_KIND_NOTIFICATION) {
      onAgentDraftChange((current) => {
        const baseDraft = current ?? agentDraft;
        return ensureNotifierPullSubscriptionDraft({
          ...baseDraft,
          avatar: baseDraft.avatar || "",
          bot_type: BOT_TYPE_NOTIFICATION,
          from_template: "",
          template_name: "",
          notifier_delivery_mode: baseDraft.notifier_delivery_mode || "webhook",
        });
      });
      return;
    }
    onAgentDraftChange((current) => {
      const baseDraft = current ?? agentDraft;
      const runtimeKindRaw = normalizeRuntimeKind(baseDraft.runtime_kind) || DEFAULT_RUNTIME_KIND;
      const runtimeKind = runtimeKindRaw === "notifier" ? DEFAULT_RUNTIME_KIND : runtimeKindRaw;
      const template = pickDefaultAgentTemplate(hubTemplates, runtimeKind, bootstrapConfig);
      return applyTemplateToDraft(
        {
          ...baseDraft,
          avatar: baseDraft.avatar || "",
          bot_type: BOT_TYPE_NORMAL,
          runtime_kind: runtimeKind,
          image: runtimeImageForKind(
            runtimeKind,
            bootstrapConfig,
            managerAgent?.image || baseDraft.default_image || "",
          ),
        },
        template,
        bootstrapConfig,
        managerAgent?.image || "",
      );
    });
    onAgentModelsReset();
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
                      readOnly={agentModalMode === "edit"}
                      disabled={agentModalMode === "edit"}
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
                {agentModalMode === "create" ? (
                  <div
                    className="workspace-tabbar agent-create-kind-tabbar"
                    role="tablist"
                    aria-label={t("createAgentKindTabAriaLabel")}
                  >
                    <Button
                      className="workspace-tab"
                      active={agentCreateBotKind === BOT_CREATE_KIND_WORKER}
                      role="tab"
                      aria-selected={agentCreateBotKind === BOT_CREATE_KIND_WORKER}
                      onClick={() => switchCreateBotKind(BOT_CREATE_KIND_WORKER)}
                    >
                      <span className="workspace-tab-copy">
                        <strong>{t("createAgentKindWorker")}</strong>
                        <small>{t("createAgentKindWorkerDescription")}</small>
                      </span>
                    </Button>
                    <Button
                      className="workspace-tab"
                      active={agentCreateBotKind === BOT_CREATE_KIND_NOTIFICATION}
                      role="tab"
                      aria-selected={agentCreateBotKind === BOT_CREATE_KIND_NOTIFICATION}
                      onClick={() => switchCreateBotKind(BOT_CREATE_KIND_NOTIFICATION)}
                    >
                      <span className="workspace-tab-copy">
                        <strong>{t("createAgentKindNotification")}</strong>
                        <small>{t("createAgentKindNotificationDescription")}</small>
                      </span>
                    </Button>
                  </div>
                ) : null}
              </div>
            </div>
          </section>
          {!isNotificationContext ? (
            <section className="profile-section">
              <div className="profile-section-heading">
                <div className="profile-section-title">{t("profileRuntimeSection")}</div>
                <p className="profile-section-description">{t("profileRuntimeSectionDescription")}</p>
              </div>
              <div className="agent-section-form">
                <div className="profile-grid profile-grid-compact agent-basics-grid">
                  {isWorkerCreate ? (
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
                        options={[
                          { value: "", label: t("templateNone") },
                          ...hubTemplates
                            .filter((item) => item.id)
                            .map((item) => ({ value: item.id || "", label: item.name || item.id || "" })),
                        ]}
                      />
                      <small className="field-hint">{t("templateHelp")}</small>
                    </label>
                  ) : null}
                  {isWorkerCreate ? (
                    <div className="agent-runtime-image-row">
                      <label className="field">
                        <span>{t("profileRuntimeKind")}</span>
                        {templateLocked ? (
                          <input
                            value={formatRuntimeKindLabel(
                              normalizeRuntimeKind(agentDraft.runtime_kind) || DEFAULT_RUNTIME_KIND,
                              t,
                            )}
                            readOnly
                            disabled
                          />
                        ) : (
                          <Select
                            value={normalizeRuntimeKind(agentDraft.runtime_kind) || DEFAULT_RUNTIME_KIND}
                            onValueChange={(value) => {
                              const runtimeKind = normalizeRuntimeKind(value);
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
                                runtime_kind: runtimeKind,
                                image: runtimeImageForKind(
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
                            options={WORKER_RUNTIME_KIND_OPTIONS.map((option) => ({
                              value: option.value,
                              label: formatRuntimeKindLabel(option.value, t),
                            }))}
                          />
                        )}
                      </label>
                      <label className="field">
                        <span>{t("agentImage")}</span>
                        <input
                          value={agentDraft.image}
                          readOnly={templateLocked}
                          disabled={templateLocked}
                          onInput={(event) => onAgentDraftChange({ ...agentDraft, image: event.currentTarget.value })}
                          placeholder={t("agentImagePlaceholder")}
                        />
                        <small className="field-hint">{t("agentImageTemplateHint")}</small>
                      </label>
                    </div>
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
          ) : (
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
          )}
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

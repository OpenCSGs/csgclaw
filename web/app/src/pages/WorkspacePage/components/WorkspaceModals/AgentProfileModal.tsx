import {
  BOT_CREATE_KIND_NOTIFICATION,
  BOT_CREATE_KIND_WORKER,
  BOT_TYPE_NORMAL,
  BOT_TYPE_NOTIFICATION,
  DEFAULT_RUNTIME_KIND,
  PROVIDERS,
  WORKER_RUNTIME_KIND_OPTIONS,
} from "@/shared/constants/agents";
import { useEffect, useRef, useState, type SetStateAction } from "react";
import {
  AgentCreateProgress,
  type AgentCreateProgressProps,
  APIKeyField,
  CLIProxyAuthControl,
  EnvKeyValueEditor,
  isBlank,
  NotifierControls,
  profileBaseURLMissing,
  requiredFieldLabel,
  RuntimeOptionsFields,
} from "@/components/business/ProfileControls";
import { Button, Select } from "@/components/ui";
import { AgentAvatarPicker } from "@/components/business/AgentAvatar";
import {
  agentCreateTemplateLocked,
  applyTemplateToDraft,
  ensureNotifierPullSubscriptionDraft,
  formatProviderLabel,
  formatRuntimeKindLabel,
  isNotificationBotDraftContext,
  normalizeAuthProviderName,
  normalizeRuntimeKind,
  normalizeTemplateSelection,
  notifierFormIsComplete,
  pickDefaultAgentTemplate,
  runtimeOptionSchemasForAgent,
  runtimeImageForKind,
  templateMatchesRuntime,
} from "@/models/agents";
import type { AgentDraft, AgentLike, RuntimeBootstrapConfig } from "@/models/agents";
import type { TranslateFn } from "@/models/conversations";
import type { LocaleCode } from "@/models/conversations";
import type { HubTemplate } from "@/models/hubWorkspace";
import type { CLIProxyAuthStatusMap } from "@/hooks/workspace/useCLIProxyAuthStatuses";
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
  agentModels?: string[];
  agentProgress?: AgentCreateProgressProps["progress"];
  authBusyProvider?: string;
  authStatuses?: CLIProxyAuthStatusMap;
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
  agentModels = [],
  agentModelBusy = false,
  authStatuses = {},
  authBusyProvider = "",
  notifierWebhookPublicOrigin = "",
  onProviderLogin,
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
            <div className="profile-section-heading">
              <div className="profile-section-title">{t("profileBasics")}</div>
              <p className="profile-section-description">{t("profileBasicsDescription")}</p>
            </div>
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
                    <div className="profile-runtime-grid">
                      <label className="field">
                        <span>{t("profileProvider")}</span>
                        <Select
                          value={agentDraft.provider}
                          onValueChange={(value) => {
                            onAgentDraftChange({ ...agentDraft, provider: value, model_id: "" });
                            onAgentModelsReset();
                          }}
                          triggerProps={{ "aria-label": t("profileProvider") }}
                          options={PROVIDERS.map((provider) => ({
                            value: provider,
                            label: formatProviderLabel(provider),
                          }))}
                        />
                      </label>
                      <label className="field">
                        {requiredFieldLabel(t("profileModel"))}
                        <Select
                          value={agentDraft.model_id}
                          required
                          onValueChange={(value) => onAgentDraftChange({ ...agentDraft, model_id: value })}
                          triggerProps={{ "aria-label": t("profileModel"), "aria-required": true }}
                          options={[
                            { value: "", label: agentModelBusy ? t("profileLoadingModels") : t("profileSelectModel") },
                            ...agentModels.map((model) => ({ value: model, label: model })),
                            ...(agentDraft.model_id && !agentModels.includes(agentDraft.model_id)
                              ? [{ value: agentDraft.model_id, label: agentDraft.model_id }]
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
                        <label className="selection-item compact-toggle-row">
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
                    <CLIProxyAuthControl
                      provider={agentDraft.provider}
                      t={t}
                      status={authStatuses[normalizeAuthProviderName(agentDraft.provider)]}
                      busy={authBusyProvider === normalizeAuthProviderName(agentDraft.provider)}
                      onLogin={onProviderLogin}
                    />
                    {agentDraft.provider === "api" ? (
                      <div className="profile-model-api-section">
                        <div className="profile-section-subtitle">{t("profileAPIProvider")}</div>
                        <div className="profile-api-grid">
                          <label className="field">
                            {requiredFieldLabel(t("profileBaseURL"))}
                            <input
                              value={agentDraft.base_url}
                              required
                              aria-required="true"
                              onInput={(event) =>
                                onAgentDraftChange({ ...agentDraft, base_url: event.currentTarget.value })
                              }
                              placeholder="https://api.openai.com/v1"
                            />
                          </label>
                          <APIKeyField
                            value={agentDraft.api_key}
                            onInput={(event) =>
                              onAgentDraftChange({ ...agentDraft, api_key: event.currentTarget.value })
                            }
                            profile={agentDraft}
                            required={!agentDraft.api_key_set}
                            t={t}
                          />
                          <label className="field span-2">
                            <span>{t("profileHeaders")}</span>
                            <textarea
                              className="compact-textarea"
                              value={agentDraft.headersText}
                              onInput={(event) =>
                                onAgentDraftChange({ ...agentDraft, headersText: event.currentTarget.value })
                              }
                            />
                          </label>
                        </div>
                      </div>
                    ) : null}
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
                  <label className="field">
                    <span>{t("profileRequestOptions")}</span>
                    <textarea
                      className="compact-json"
                      value={agentDraft.requestOptionsText}
                      onInput={(event) =>
                        onAgentDraftChange({ ...agentDraft, requestOptionsText: event.currentTarget.value })
                      }
                    />
                  </label>
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
                : !agentDraft.model_id || profileBaseURLMissing(agentDraft))
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

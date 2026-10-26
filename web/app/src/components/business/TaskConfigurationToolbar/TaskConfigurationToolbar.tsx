import { useState } from "react";
import { Check, ChevronDown, Gauge, ShieldCheck } from "lucide-react";
import { PopoverRoot, PopoverTrigger, PopoverContent } from "@/components/ui";
import { ReasoningEffortPicker } from "@/components/business/ReasoningEffortPicker";
import { reasoningEffortLabel } from "@/components/business/ProfileControls";
import { useOptionalWorkspaceControllerContext } from "@/hooks/workspace";
import {
  agentProfileConfig,
  agentRuntimeOptions,
  agentRuntimeKind,
  runtimeOptionSchemasForAgent,
  runtimeOptionValueForPath,
  localizedRuntimeOptionChoiceLabel,
  type AgentLike,
} from "@/models/agents";
import { modelProviderSelectOptionsFromCatalog, modelReasoningEfforts } from "@/models/modelProviders";
import { TaskModelPicker } from "./TaskModelPicker";
import { TaskDirectoryPicker } from "./TaskDirectoryPicker";
import { supportsTaskConfiguration, type TaskConfiguration } from "@/models/taskConfiguration";
import styles from "./TaskConfigurationToolbar.module.css";

type Menu = "" | "reasoning" | "mode";
export function TaskConfigurationToolbar({
  agent,
  showModel = true,
  showDirectory = true,
  disabled = false,
  configuration = {},
  directoryLocked = false,
  onChange,
  t: translate,
}: {
  agent?: AgentLike;
  showModel?: boolean;
  showDirectory?: boolean;
  disabled?: boolean;
  configuration?: TaskConfiguration;
  directoryLocked?: boolean;
  onChange?: (value: TaskConfiguration) => void | Promise<void>;
  t?: (key: string) => string;
}) {
  const controller = useOptionalWorkspaceControllerContext();
  const t = translate || controller?.t || ((key: string) => key);
  const profile = agentProfileConfig(agent);
  const options = agentRuntimeOptions(agent);
  const reasoning = configuration.reasoning_effort || profile?.reasoning_effort || "auto";
  const [open, setOpen] = useState<Menu>("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [reasoningPreview, setReasoningPreview] = useState<string>();
  const editable = supportsTaskConfiguration(agent) && Boolean(onChange);
  const blocked = disabled || saving;
  const providers = modelProviderSelectOptionsFromCatalog(controller?.modelProviders);
  const effectiveProvider = configuration.model_provider_id || profile?.model_provider_id || "";
  const effectiveModel = configuration.model_id || profile?.model_id || "";
  const supportedEfforts = modelReasoningEfforts(providers, effectiveProvider, effectiveModel);
  const schema = runtimeOptionSchemasForAgent(agentRuntimeKind(agent), agent).find(
    (option) =>
      option.type === "select" && ["execution_mode", "permission_mode"].includes(String(option.path || option.key)),
  );
  const mode =
    configuration.execution_mode ||
    (schema ? runtimeOptionValueForPath(options, schema.path || schema.key, schema.default_value || "") : "");
  const labelForMode = (value: string) =>
    localizedRuntimeOptionChoiceLabel(
      schema?.choices?.find((choice) => choice.value === value),
      controller?.sidebarProps?.locale || "zh",
    ) || value;
  const labelForReasoning = (value: string) =>
    value === "auto" ? t("iaReasoningDefault") : reasoningEffortLabel(t, value);
  const toggle = (kind: Menu, value: boolean) => {
    setOpen(value ? kind : "");
    setError("");
    setReasoningPreview(undefined);
  };
  async function change(patch: TaskConfiguration, keepOpen = false, propagateError = false) {
    if (!editable || blocked) return;
    setSaving(true);
    setError("");
    try {
      await onChange?.({ ...configuration, ...patch });
      if (!keepOpen) setOpen("");
    } catch (cause) {
      if (propagateError) throw cause;
      setError(cause instanceof Error ? cause.message : t("sendFailed"));
    } finally {
      setSaving(false);
    }
  }
  async function commitReasoning(value: string) {
    setReasoningPreview(value);
    await change({ reasoning_effort: value }, true);
    setReasoningPreview(undefined);
  }
  const hint = (
    <p className={styles.hint}>{t(editable ? "taskConfigurationSessionOnly" : "taskConfigurationUnsupported")}</p>
  );
  const errorMessage = error ? (
    <p role="alert" className={styles.error}>
      {error}
    </p>
  ) : null;
  function row(
    label: string,
    selected: boolean,
    action: () => void,
    detail?: string,
    unavailable = false,
    avatar?: string,
  ) {
    return (
      <button
        key={label}
        type="button"
        className={`${styles.row} ${selected ? styles.active : ""}`}
        aria-pressed={selected}
        disabled={blocked || unavailable || !editable}
        onClick={action}
      >
        {avatar ? <img src={avatar} alt="" /> : null}
        <span>
          {label}
          {detail ? <small>{detail}</small> : null}
        </span>
        {selected ? <Check size={15} /> : null}
      </button>
    );
  }
  return (
    <div className={styles.root} title={!editable ? t("taskConfigurationUnsupported") : undefined}>
      {showDirectory ? (
        <TaskDirectoryPicker
          value={configuration.workspace_dir || ""}
          defaultDirectory={String(options.local_workspace_dir || "")}
          locked={directoryLocked}
          disabled={blocked || !editable}
          onChoose={(path) => change({ workspace_dir: path }, false, true)}
          t={t}
        />
      ) : null}
      {showModel && schema ? (
        <PopoverRoot open={open === "mode"} onOpenChange={(value) => toggle("mode", value)}>
          <PopoverTrigger asChild>
            <button type="button" disabled={blocked} className={styles.trigger} aria-label={t("iaRunModeTitle")}>
              <ShieldCheck size={15} />
              <span>{labelForMode(mode)}</span>
              <ChevronDown size={12} />
            </button>
          </PopoverTrigger>
          <PopoverContent className={styles.popover} side="top" aria-label={t("iaRunModeTitle")}>
            {row(t("iaRunModeFollowAgent"), !configuration.execution_mode, () => void change({ execution_mode: "" }))}
            {(schema.choices || []).map((choice) =>
              row(
                labelForMode(String(choice.value || "")),
                configuration.execution_mode === choice.value,
                () => void change({ execution_mode: String(choice.value || "") }),
              ),
            )}
            {errorMessage}
            {hint}
          </PopoverContent>
        </PopoverRoot>
      ) : null}
      {showModel ? (
        <div className={styles.modelGroup}>
          <TaskModelPicker
            providers={providers}
            providerID={configuration.model_provider_id || profile?.model_provider_id || ""}
            modelID={configuration.model_id || profile?.model_id || ""}
            agentModelID={profile?.model_id || ""}
            agentProviderID={profile?.model_provider_id || ""}
            followDefault={!configuration.model_id}
            disabled={blocked || !editable}
            onFollowDefault={() => change({ model_provider_id: "", model_id: "", reasoning_effort: "" }, false, true)}
            onChoose={(id, model) =>
              change({ model_provider_id: id, model_id: model, reasoning_effort: "auto" }, false, true)
            }
            t={t}
          />
          <PopoverRoot open={open === "reasoning"} onOpenChange={(value) => toggle("reasoning", value)}>
            <PopoverTrigger asChild>
              <button type="button" disabled={blocked} className={styles.trigger} aria-label={t("iaReasoningTitle")}>
                <Gauge size={15} />
                <span>{labelForReasoning(configuration.reasoning_effort ? reasoning : "auto")}</span>
                <ChevronDown size={12} />
              </button>
            </PopoverTrigger>
            <PopoverContent className={styles.popover} side="top" align="end" aria-label={t("iaReasoningTitle")}>
              <ReasoningEffortPicker
                compact
                value={reasoningPreview ?? configuration.reasoning_effort ?? ""}
                agentEffort={profile?.reasoning_effort ?? undefined}
                supportedEfforts={supportedEfforts}
                disabled={!editable || blocked}
                onValueChange={setReasoningPreview}
                onValueCommit={(value) => void commitReasoning(value)}
                t={t}
              />
              {errorMessage}
            </PopoverContent>
          </PopoverRoot>
        </div>
      ) : null}
    </div>
  );
}

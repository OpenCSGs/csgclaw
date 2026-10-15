import { useState } from "react";
import { Check, ChevronDown, Folder, Gauge, Settings2, ShieldCheck } from "lucide-react";
import { Button, PopoverRoot, PopoverTrigger, PopoverContent, TextInput } from "@/components/ui";
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
import { modelProviderSelectOptionsFromCatalog } from "@/models/modelProviders";
import { ConnectedModelPicker } from "@/components/business/ConnectedModelPicker";
import { supportsTaskConfiguration, type TaskConfiguration } from "@/models/taskConfiguration";
import { pickLocalDirectoryPath } from "@/components/business/ProfileControls/runtimeOptionDirectoryPicker";
import styles from "./TaskConfigurationToolbar.module.css";

type Menu = "" | "directory" | "model" | "reasoning" | "mode";
export function TaskConfigurationToolbar({
  agent,
  showModel = true,
  disabled = false,
  configuration = {},
  directoryLocked = false,
  onChange,
  t: translate,
}: {
  agent?: AgentLike;
  showModel?: boolean;
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
  const directory = configuration.workspace_dir ?? String(options.local_workspace_dir || "");
  const reasoning = configuration.reasoning_effort || profile?.reasoning_effort || "auto";
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState<Menu>("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [reasoningPreview, setReasoningPreview] = useState<string>();
  const editable = supportsTaskConfiguration(agent) && Boolean(onChange);
  const blocked = disabled || saving;
  const providers = modelProviderSelectOptionsFromCatalog(controller?.modelProviders);
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
    setQuery("");
    setError("");
    setReasoningPreview(undefined);
  };
  async function change(patch: TaskConfiguration, keepOpen = false) {
    if (!editable || blocked) return;
    setSaving(true);
    setError("");
    try {
      await onChange?.({ ...configuration, ...patch });
      if (!keepOpen) setOpen("");
    } catch (cause) {
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
  const editAgent = () => {
    setOpen("");
    if (agent && controller?.ready) void controller.openAgentSettings?.(agent);
  };
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
    <div className={styles.root}>
      <PopoverRoot open={open === "directory"} onOpenChange={(value) => toggle("directory", value)}>
        <PopoverTrigger asChild>
          <button type="button" disabled={blocked} className={styles.trigger} aria-label={t("iaWorkdirTitle")}>
            <Folder size={16} />
            <span title={directory}>
              {directory
                ? directory
                    .replace(/[\\/]+$/, " ")
                    .trim()
                    .split(/[\\/]/)
                    .at(-1)
                : t("iaWorkdirDefault")}
            </span>
            <ChevronDown size={12} />
          </button>
        </PopoverTrigger>
        <PopoverContent className={styles.popover} side="top" aria-label={t("iaWorkdirTitle")}>
          <p className={styles.hint}>{directory || t("iaWorkdirDefaultHint")}</p>
          {directoryLocked ? (
            <p className={styles.hint}>{t("taskConfigurationDirectoryLocked")}</p>
          ) : (
            <>
              <label className={styles.directoryInput}>
                {t("iaWorkdirTitle")}
                <TextInput
                  size="sm"
                  aria-label={t("taskConfigurationDirectoryPath")}
                  placeholder="/path/to/workspace"
                  value={query}
                  disabled={!editable || blocked}
                  onChange={(event) => setQuery(event.target.value)}
                />
              </label>
              <Button
                size="sm"
                disabled={!editable || blocked || !query.trim()}
                onClick={() => void change({ workspace_dir: query.trim() })}
              >
                {t("taskConfigurationApply")}
              </Button>
              <Button
                variant="tertiaryGray"
                size="sm"
                disabled={!editable || blocked}
                onClick={() => {
                  void pickLocalDirectoryPath()
                    .then((path) => {
                      if (path) return change({ workspace_dir: path });
                    })
                    .catch((cause) => setError(String(cause)));
                }}
              >
                {t("taskConfigurationChooseDirectory")}
              </Button>
              {row(t("iaWorkdirDefault"), !configuration.workspace_dir, () => void change({ workspace_dir: "" }))}
            </>
          )}
          {errorMessage}
          {hint}
        </PopoverContent>
      </PopoverRoot>
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
          <ConnectedModelPicker
            providers={providers}
            providerID={configuration.model_provider_id || profile?.model_provider_id || ""}
            modelID={configuration.model_id || profile?.model_id || ""}
            followDefault={!configuration.model_id}
            disabled={blocked || !editable}
            compact
            onFollowDefault={() => change({ model_provider_id: "", model_id: "", reasoning_effort: "" })}
            onChoose={(id, model) => change({ model_provider_id: id, model_id: model, reasoning_effort: "auto" })}
            t={controller?.t || ((key) => t(key))}
            footer={
              <>
                {errorMessage}
                {hint}
                {agent && controller?.ready ? (
                  <Button variant="tertiaryGray" size="sm" onClick={editAgent}>
                    <Settings2 size={15} />
                    {t("taskRuntimeEditAgent")}
                  </Button>
                ) : null}
              </>
            }
          />
          <PopoverRoot open={open === "reasoning"} onOpenChange={(value) => toggle("reasoning", value)}>
            <PopoverTrigger asChild>
              <button type="button" disabled={blocked} className={styles.trigger} aria-label={t("iaReasoningTitle")}>
                <Gauge size={15} />
                <span>{labelForReasoning(reasoning)}</span>
                <ChevronDown size={12} />
              </button>
            </PopoverTrigger>
            <PopoverContent className={styles.popover} side="top" align="end" aria-label={t("iaReasoningTitle")}>
              <ReasoningEffortPicker
                value={reasoningPreview ?? configuration.reasoning_effort ?? ""}
                agentEffort={profile?.reasoning_effort ?? undefined}
                disabled={!editable || blocked}
                onValueChange={setReasoningPreview}
                onValueCommit={(value) => void commitReasoning(value)}
                t={t}
              />
              {errorMessage}
              {hint}
            </PopoverContent>
          </PopoverRoot>
        </div>
      ) : null}
    </div>
  );
}

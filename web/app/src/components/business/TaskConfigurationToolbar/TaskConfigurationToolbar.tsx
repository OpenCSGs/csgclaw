import { useState } from "react";
import { Check, ChevronDown, Folder, FolderPlus, Gauge, Home, Search, Settings2, ShieldCheck } from "lucide-react";
import { Button, PopoverClose, PopoverRoot, PopoverTrigger, PopoverContent, TextInput, Tooltip } from "@/components/ui";
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
const RECENT_DIRECTORIES_KEY = "csgclaw.recent-work-directories";
function readRecentDirectories(): string[] {
  try {
    const value: unknown = JSON.parse(localStorage.getItem(RECENT_DIRECTORIES_KEY) || "[]");
    return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string" && !!item) : [];
  } catch {
    return [];
  }
}
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
  const [recentDirectories, setRecentDirectories] = useState(readRecentDirectories);
  const [open, setOpen] = useState<Menu>("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [reasoningPreview, setReasoningPreview] = useState<string>();
  const availableDirectories = configuration.workspace_dir
    ? [configuration.workspace_dir, ...recentDirectories.filter((path) => path !== configuration.workspace_dir)]
    : recentDirectories;
  const matchingDirectories = availableDirectories.filter((path) =>
    path.toLowerCase().includes(query.trim().toLowerCase()),
  );
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
      if (patch.workspace_dir) {
        const next = [
          patch.workspace_dir,
          ...readRecentDirectories().filter((path) => path !== patch.workspace_dir),
        ].slice(0, 20);
        setRecentDirectories(next);
        try {
          localStorage.setItem(RECENT_DIRECTORIES_KEY, JSON.stringify(next));
        } catch {
          /* Storage may be unavailable. */
        }
      }
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
        <PopoverContent
          className={`${styles.popover} ${styles.directoryPopover}`}
          side="top"
          aria-label={t("iaWorkdirTitle")}
          onOpenAutoFocus={(event) => event.preventDefault()}
        >
          {directoryLocked ? (
            <>
              <p className={styles.hint}>{directory || t("iaWorkdirDefaultHint")}</p>
              <p className={styles.hint}>{t("taskConfigurationDirectoryLocked")}</p>
            </>
          ) : (
            <>
              <div className={styles.directorySearch}>
                <Search size={16} aria-hidden="true" />
                <TextInput
                  size="sm"
                  aria-label={t("taskDirectorySearch")}
                  placeholder={t("taskDirectorySearch")}
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
              </div>
              {(!query || t("iaWorkdirDefault").toLowerCase().includes(query.toLowerCase())) && (
                <button
                  type="button"
                  className={`${styles.directoryOption} ${!configuration.workspace_dir ? styles.active : ""}`}
                  disabled={!editable || blocked}
                  onClick={() => void change({ workspace_dir: "" })}
                >
                  <Home size={16} aria-hidden="true" />
                  <span>
                    <strong>{t("iaWorkdirDefault")}</strong>
                    <small>{t("iaWorkdirDefaultHint")}</small>
                  </span>
                  {!configuration.workspace_dir && <Check size={16} aria-hidden="true" />}
                </button>
              )}
              {matchingDirectories.length > 0 && (
                <div className={styles.directoryList}>
                  <div className={styles.directoryGroupTitle}>{t("taskDirectoryRecent")}</div>
                  {matchingDirectories.map((path) => {
                    const trimmed = path.replace(/[\\/]+$/, "");
                    const separator = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
                    const prefix = separator >= 0 ? trimmed.slice(0, separator + 1) : "";
                    const current = separator >= 0 ? trimmed.slice(separator + 1) : trimmed;
                    return (
                      <Tooltip key={path} content={path}>
                        <button
                          type="button"
                          className={`${styles.directoryOption} ${configuration.workspace_dir === path ? styles.active : ""}`}
                          disabled={!editable || blocked}
                          onClick={() => void change({ workspace_dir: path })}
                        >
                          <Folder size={16} aria-hidden="true" />
                          <span>
                            <strong>
                              {path
                                .replace(/[\\/]+$/, "")
                                .split(/[\\/]/)
                                .at(-1) || path}
                            </strong>
                            <small className={styles.directoryPath}>
                              <span className={styles.directoryPrefix}>{prefix}</span>
                              <span className={styles.directoryCurrent}>{current || path}</span>
                            </small>
                          </span>
                          {configuration.workspace_dir === path && <Check size={16} aria-hidden="true" />}
                        </button>
                      </Tooltip>
                    );
                  })}
                </div>
              )}
              <div className={styles.directoryFooter}>
                <button
                  type="button"
                  disabled={!editable || blocked}
                  onClick={() => {
                    void pickLocalDirectoryPath()
                      .then((path) => {
                        if (path) return change({ workspace_dir: path });
                      })
                      .catch((cause) => setError(cause instanceof Error ? cause.message : String(cause)));
                  }}
                >
                  <FolderPlus size={16} aria-hidden="true" />
                  {t("taskDirectoryAdd")}
                </button>
              </div>
            </>
          )}
          {errorMessage}
          {directoryLocked ? hint : null}
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
            showConnectionActions={false}
            onFollowDefault={() => change({ model_provider_id: "", model_id: "", reasoning_effort: "" })}
            onChoose={(id, model) => change({ model_provider_id: id, model_id: model, reasoning_effort: "auto" })}
            t={controller?.t || ((key) => t(key))}
            footer={
              <div className={styles.modelFooter}>
                {errorMessage}
                {controller?.ready && controller.sidebarProps.onCreateModelProvider ? (
                  <PopoverClose asChild>
                    <Button
                      variant="linkColor"
                      size="sm"
                      onClick={() =>
                        controller.sidebarProps.onCreateModelProvider?.({
                          preset: "custom",
                          onConnected: (provider) => {
                            if (provider.models[0])
                              void change({
                                model_provider_id: provider.id,
                                model_id: provider.models[0],
                                reasoning_effort: "auto",
                              });
                          },
                        })
                      }
                    >
                      <Settings2 size={15} />
                      {t("iaModelConfigure")}
                    </Button>
                  </PopoverClose>
                ) : null}
              </div>
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
                scopeNote={t(editable ? "taskConfigurationSessionOnly" : "taskConfigurationUnsupported")}
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

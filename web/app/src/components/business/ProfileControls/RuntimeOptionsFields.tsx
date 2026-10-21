import {
  localizedRuntimeOptionChoiceDescription,
  localizedRuntimeOptionChoiceLabel,
  localizedRuntimeOptionDescription,
  localizedRuntimeOptionLabel,
  runtimeOptionValueForPath,
  setRuntimeOptionValue,
  type AgentDraft,
  type RuntimeOptionChoice,
  type RuntimeOptionSchema,
} from "@/models/agents";
import { Button } from "@/components/ui/Button/Button";
import { Select, Tooltip } from "@/components/ui";
import type { LocaleCode } from "@/models/conversations";
import { pickLocalDirectoryPath, rendererDirectoryPickerAvailable } from "./runtimeOptionDirectoryPicker";

export type RuntimeOptionsFieldsProps = {
  draft: AgentDraft;
  locale: LocaleCode;
  schemas?: RuntimeOptionSchema[] | null;
  onDraftChange: (draft: AgentDraft) => void;
  embedded?: boolean;
  directoryPickerAvailable?: boolean;
  showExecutionMode?: boolean;
};

function directoryPickerLabel(locale: LocaleCode): string {
  return locale === "zh" ? "选择目录" : "Choose directory";
}

function clearFieldLabel(locale: LocaleCode): string {
  return locale === "zh" ? "清空" : "Clear";
}

function choiceForValue(choices: RuntimeOptionChoice[] | null | undefined, value: string) {
  return choices?.find((choice) => String(choice.value ?? "").trim() === value);
}

function ExecutionModeHelp({ locale }: { locale: LocaleCode }) {
  const zh = locale === "zh";
  const rows = zh
    ? [
        ["读取 Skill 附属文件或执行脚本", "允许", "禁止"],
        ["读取工作目录或系统文件", "允许", "禁止"],
        ["查看环境变量或凭据", "按运行环境", "禁止"],
        ["执行命令或启停服务", "允许", "禁止"],
        ["修改文件或外部业务数据", "按工具权限", "禁止"],
        ["请求提升权限", "按运行策略", "禁止"],
        ["对话问答、分析与总结", "允许", "允许"],
        ["读取对话中直接提供的内容", "允许", "允许"],
        ["加载已分配 Skill 的主说明", "允许", "允许"],
        ["查询只读 MCP 数据源", "按工具权限", "允许"],
      ]
    : [
        ["Read Skill resources or run scripts", "Allowed", "Blocked"],
        ["Read workspace or system files", "Allowed", "Blocked"],
        ["Inspect environment or credentials", "Per runtime", "Blocked"],
        ["Run commands or services", "Allowed", "Blocked"],
        ["Modify files or external data", "Per tool", "Blocked"],
        ["Request elevated access", "Per policy", "Blocked"],
        ["Conversation and analysis", "Allowed", "Allowed"],
        ["Read content provided in chat", "Allowed", "Allowed"],
        ["Load assigned Skill instructions", "Allowed", "Allowed"],
        ["Query read-only MCP data", "Per tool", "Allowed"],
      ];
  return (
    <Tooltip
      content={
        <div className="execution-mode-help">
          <strong>{zh ? "不同模式权限" : "Mode permissions"}</strong>
          <table>
            <thead>
              <tr>
                <th>{zh ? "能力" : "Capability"}</th>
                <th>{zh ? "标准模式" : "Standard"}</th>
                <th>{zh ? "只读模式" : "Read-only"}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row, index) => (
                <tr key={row[0]}>
                  <td>{row[0]}</td>
                  <td>{row[1]}</td>
                  <td className={index < 6 ? "blocked" : "allowed"}>{row[2]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      }
      contentProps={{ side: "top", align: "start", className: "execution-mode-tooltip" }}
    >
      <button
        type="button"
        className="field-help-trigger"
        aria-label={zh ? "查看运行模式权限" : "View execution mode permissions"}
      >
        ?
      </button>
    </Tooltip>
  );
}

export function RuntimeOptionsFields({
  draft,
  locale,
  schemas = [],
  onDraftChange,
  embedded = false,
  directoryPickerAvailable = true,
  showExecutionMode = false,
}: RuntimeOptionsFieldsProps) {
  if (!Array.isArray(schemas) || schemas.length === 0) {
    return null;
  }
  const showDirectoryPicker = directoryPickerAvailable || rendererDirectoryPickerAvailable();

  const fields = schemas.map((schema) => {
    const path = String(schema.path ?? "").trim();
    if (!path || (path === "execution_mode" && !showExecutionMode)) {
      return null;
    }
    const label = localizedRuntimeOptionLabel(schema, locale);
    const description = localizedRuntimeOptionDescription(schema, locale);
    const inputValue = runtimeOptionValueForPath(draft.runtime_options, path, String(schema.default_value ?? ""));
    const isDirectory = schema.type === "directory";
    const isSelect = schema.type === "select";
    const placeholder = isDirectory ? "/path/to/workspace" : "";
    if (isSelect) {
      const resolvedValue = inputValue || String(schema.default_value ?? schema.options?.[0] ?? "");
      return (
        <div key={String(schema.key ?? path)} className="field span-2">
          <div className="field-label-with-help">
            <span>{label}</span>
            {path === "execution_mode" ? <ExecutionModeHelp locale={locale} /> : null}
          </div>
          <Select
            value={resolvedValue}
            onValueChange={(value) =>
              onDraftChange({
                ...draft,
                runtime_options: setRuntimeOptionValue(draft.runtime_options, path, value),
              })
            }
            triggerProps={{ "aria-label": label }}
            options={(schema.options || []).map((value) => ({
              value,
              label: localizedRuntimeOptionChoiceLabel(choiceForValue(schema.choices, value), locale) || value,
            }))}
          />
          <span className="field-hint">
            {localizedRuntimeOptionChoiceDescription(choiceForValue(schema.choices, resolvedValue), locale) ||
              description}
          </span>
        </div>
      );
    }
    return (
      <label key={String(schema.key ?? path)} className="field span-2">
        <span>{label}</span>
        <div className={isDirectory ? "runtime-option-input-row" : undefined}>
          <input
            value={inputValue}
            required={Boolean(schema.required)}
            aria-required={schema.required ? "true" : undefined}
            placeholder={placeholder}
            onInput={(event) =>
              onDraftChange({
                ...draft,
                runtime_options: setRuntimeOptionValue(draft.runtime_options, path, event.currentTarget.value),
              })
            }
          />
          {isDirectory ? (
            <>
              {showDirectoryPicker ? (
                <Button
                  variant="secondaryGray"
                  size="md"
                  className="runtime-option-action"
                  onClick={async () => {
                    const pickedPath = await pickLocalDirectoryPath();
                    if (!pickedPath) {
                      return;
                    }
                    onDraftChange({
                      ...draft,
                      runtime_options: setRuntimeOptionValue(draft.runtime_options, path, pickedPath),
                    });
                  }}
                >
                  {directoryPickerLabel(locale)}
                </Button>
              ) : null}
              <Button
                variant="secondaryGray"
                size="md"
                className="runtime-option-action"
                onClick={() =>
                  onDraftChange({
                    ...draft,
                    runtime_options: setRuntimeOptionValue(draft.runtime_options, path, ""),
                  })
                }
              >
                {clearFieldLabel(locale)}
              </Button>
            </>
          ) : null}
        </div>
        {description ? <span className="field-hint">{description}</span> : null}
      </label>
    );
  });

  if (embedded) {
    return <>{fields}</>;
  }

  return <div className="profile-grid-compact">{fields}</div>;
}

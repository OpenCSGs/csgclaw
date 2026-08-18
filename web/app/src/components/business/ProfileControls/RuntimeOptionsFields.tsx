import {
  localizedRuntimeOptionDescription,
  localizedRuntimeOptionLabel,
  runtimeOptionValueForPath,
  setRuntimeOptionValue,
  type AgentDraft,
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
};

function directoryPickerLabel(locale: LocaleCode): string {
  return locale === "zh" ? "选择目录" : "Choose directory";
}

function clearFieldLabel(locale: LocaleCode): string {
  return locale === "zh" ? "清空" : "Clear";
}

const EXECUTION_MODE_PATH = "execution_mode";
const EXECUTION_MODE_READ_ONLY = "read_only";

function executionModeLabel(value: string, locale: LocaleCode): string {
  if (value === EXECUTION_MODE_READ_ONLY) {
    return locale === "zh" ? "只读模式" : "Read-only mode";
  }
  return locale === "zh" ? "标准模式" : "Standard mode";
}

function executionModeHint(value: string, locale: LocaleCode): string {
  if (value === EXECUTION_MODE_READ_ONLY) {
    return locale === "zh"
      ? "仅可分析对话内容、加载已分配 Skill 的主说明并查询只读数据源；不能读取本地文件、环境变量或执行命令。"
      : "Can only analyze conversation content, load assigned Skill instructions, and query read-only data sources; cannot read local files, inspect environment variables, or run commands.";
  }
  return locale === "zh"
    ? "可读取和修改数据，并使用运行环境允许的工具。"
    : "Can read and modify data and use tools allowed by the runtime environment.";
}

function ExecutionModeHelp({ locale }: { locale: LocaleCode }) {
  const rows =
    locale === "zh"
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
          <strong>{locale === "zh" ? "不同模式权限" : "Mode permissions"}</strong>
          <table>
            <thead>
              <tr>
                <th>{locale === "zh" ? "能力" : "Capability"}</th>
                <th>{locale === "zh" ? "标准模式" : "Standard"}</th>
                <th>{locale === "zh" ? "只读模式" : "Read-only"}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row[0]}>
                  <td>{row[0]}</td>
                  <td>{row[1]}</td>
                  <td className={row[2] === "禁止" || row[2] === "Blocked" ? "blocked" : "allowed"}>{row[2]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      }
      contentProps={{ side: "bottom", align: "start", className: "execution-mode-tooltip" }}
    >
      <button
        type="button"
        className="field-help-trigger"
        aria-label={locale === "zh" ? "查看运行模式权限" : "View execution mode permissions"}
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
}: RuntimeOptionsFieldsProps) {
  if (!Array.isArray(schemas) || schemas.length === 0) {
    return null;
  }
  const showDirectoryPicker = directoryPickerAvailable || rendererDirectoryPickerAvailable();

  const fields = schemas.map((schema) => {
    const path = String(schema.path ?? "").trim();
    if (!path) {
      return null;
    }
    const label = localizedRuntimeOptionLabel(schema, locale);
    const description = localizedRuntimeOptionDescription(schema, locale);
    const inputValue = runtimeOptionValueForPath(draft.runtime_options, path, String(schema.default_value ?? ""));
    const isDirectory = schema.type === "directory";
    const isSelect = schema.type === "select";
    const isExecutionMode = path === EXECUTION_MODE_PATH;
    const placeholder = isDirectory ? "/path/to/workspace" : "";
    if (isSelect) {
      const resolvedValue = inputValue || String(schema.default_value ?? schema.options?.[0] ?? "");
      return (
        <div key={String(schema.key ?? path)} className="field span-2">
          <div className="field-label-with-help">
            <span>{label}</span>
            {isExecutionMode ? <ExecutionModeHelp locale={locale} /> : null}
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
              label: isExecutionMode ? executionModeLabel(value, locale) : value,
            }))}
          />
          <span className="field-hint">{isExecutionMode ? executionModeHint(resolvedValue, locale) : description}</span>
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

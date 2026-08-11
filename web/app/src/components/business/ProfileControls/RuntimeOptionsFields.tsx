import {
  localizedRuntimeOptionDescription,
  localizedRuntimeOptionLabel,
  runtimeOptionValueForPath,
  setRuntimeOptionValue,
  type AgentDraft,
  type RuntimeOptionSchema,
} from "@/models/agents";
import { Button } from "@/components/ui/Button/Button";
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
    const inputValue = runtimeOptionValueForPath(draft.runtime_options, path);
    const isDirectory = schema.type === "directory";
    const placeholder = isDirectory ? "/path/to/workspace" : "";
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

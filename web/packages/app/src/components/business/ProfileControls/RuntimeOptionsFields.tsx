import profileFormLayoutStyles from "@/components/business/ProfileControls/ProfileFormLayout.module.css";
import profileControlsStyles from "./ProfileControls.module.css";
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
import { Button } from "csgclaw-ui";
import { Field, TextInput, Select } from "@/components/ui";
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

function choiceForValue(choices: RuntimeOptionChoice[] | null | undefined, value: string) {
  return choices?.find((choice) => String(choice.value ?? "").trim() === value);
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
    if (!path || path === "execution_mode") {
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
        <Field
          key={String(schema.key ?? path)}
          className={`span-2 ${profileFormLayoutStyles["span-2"]}`}
          label={label}
          hint={
            localizedRuntimeOptionChoiceDescription(choiceForValue(schema.choices, resolvedValue), locale) ||
            description
          }
        >
          <Select
            value={resolvedValue}
            onValueChange={(value) =>
              onDraftChange({
                ...draft,
                runtime_options: setRuntimeOptionValue(draft.runtime_options, path, value),
              })
            }
            options={(schema.options || []).map((value) => ({
              value,
              label: localizedRuntimeOptionChoiceLabel(choiceForValue(schema.choices, value), locale) || value,
            }))}
          />
        </Field>
      );
    }
    return (
      <Field
        as={isDirectory ? "div" : undefined}
        label={label}
        hint={description}
        required={Boolean(schema.required)}
        key={String(schema.key ?? path)}
        className={`span-2 ${profileFormLayoutStyles["span-2"]}`}
      >
        <div className={isDirectory ? profileControlsStyles["runtime-option-input-row"] : undefined}>
          <TextInput
            value={inputValue}
            required={Boolean(schema.required)}
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
                  className={profileControlsStyles["runtime-option-action"]}
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
                className={profileControlsStyles["runtime-option-action"]}
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
      </Field>
    );
  });

  if (embedded) {
    return <>{fields}</>;
  }

  return <div className={`profile-grid-compact ${profileFormLayoutStyles["profile-grid-compact"]}`}>{fields}</div>;
}

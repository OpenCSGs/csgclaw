import envKeyValueEditorStyles from "./EnvKeyValueEditor.module.css";
import { Button, Field, TextInput, Tooltip } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import type { EnvKeyValueRow, Translator } from "./types";

export type EnvKeyValueEditorProps = {
  className?: string;
  onChange: (rows: EnvKeyValueRow[]) => void;
  rows?: EnvKeyValueRow[];
  t: Translator;
};

export function EnvKeyValueEditor({ className, rows = [], t, onChange }: EnvKeyValueEditorProps) {
  const items = rows.length ? rows : [{ key: "", value: "" }];
  function update(index: number, patch: Partial<EnvKeyValueRow>) {
    onChange(items.map((row, rowIndex) => (rowIndex === index ? { ...row, ...patch } : row)));
  }
  function remove(index: number) {
    const next = items.filter((_, rowIndex) => rowIndex !== index);
    onChange(next.length ? next : [{ key: "", value: "" }]);
  }
  return (
    <fieldset className={classNames(envKeyValueEditorStyles.group, className)}>
      <legend className={envKeyValueEditorStyles.legend}>{t("profileEnv")}</legend>
      <div className="env-editor">
        {items.map((row, index) => (
          <div
            key={index}
            className={classNames(`env-row ${envKeyValueEditorStyles["env-row"]}`, row.required ? "required" : "")}
          >
            <Field className={classNames("env-key-cell", envKeyValueEditorStyles.cell)}>
              <TextInput
                aria-label={t("profileEnvKey")}
                value={row.key}
                placeholder={t("profileEnvKey")}
                required={row.required}
                aria-required={row.required ? "true" : undefined}
                onInput={(event) => update(index, { key: event.currentTarget.value })}
              />
              {row.required ? (
                <span className={`env-required-star ${envKeyValueEditorStyles.requiredMark}`} aria-hidden="true">
                  *
                </span>
              ) : null}
            </Field>
            <Field className={envKeyValueEditorStyles.cell}>
              <TextInput
                aria-label={t("profileEnvValue")}
                value={row.value}
                placeholder={t("profileEnvValue")}
                required={row.required}
                aria-required={row.required ? "true" : undefined}
                onInput={(event) => update(index, { value: event.currentTarget.value })}
              />
            </Field>
            <Tooltip content={t("profileEnvRemove")}>
              <span>
                <Button
                  variant="ghost"
                  className={`env-remove-button ${envKeyValueEditorStyles["env-remove-button"]}`}
                  aria-label={t("profileEnvRemove")}
                  disabled={row.required}
                  onClick={() => remove(index)}
                >
                  ×
                </Button>
              </span>
            </Tooltip>
          </div>
        ))}
        <Button className="env-add-button" onClick={() => onChange([...items, { key: "", value: "" }])}>
          {t("profileEnvAdd")}
        </Button>
      </div>
    </fieldset>
  );
}

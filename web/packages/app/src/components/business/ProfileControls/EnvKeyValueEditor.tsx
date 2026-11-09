import { Button, TextInput, Tooltip } from "@/components/ui";
import { classNames } from "@/shared/lib/classNames";
import type { EnvKeyValueRow, Translator } from "./types";

export type EnvKeyValueEditorProps = {
  onChange: (rows: EnvKeyValueRow[]) => void;
  rows?: EnvKeyValueRow[];
  t: Translator;
};

export function EnvKeyValueEditor({ rows = [], t, onChange }: EnvKeyValueEditorProps) {
  const items = rows.length ? rows : [{ key: "", value: "" }];
  function update(index: number, patch: Partial<EnvKeyValueRow>) {
    onChange(items.map((row, rowIndex) => (rowIndex === index ? { ...row, ...patch } : row)));
  }
  function remove(index: number) {
    const next = items.filter((_, rowIndex) => rowIndex !== index);
    onChange(next.length ? next : [{ key: "", value: "" }]);
  }
  return (
    <div className="env-editor">
      {items.map((row, index) => (
        <div key={index} className={classNames("env-row", row.required ? "required" : "")}>
          <div className="env-key-cell">
            <TextInput
              value={row.key}
              placeholder={t("profileEnvKey")}
              required={row.required}
              aria-required={row.required ? "true" : undefined}
              onInput={(event) => update(index, { key: event.currentTarget.value })}
            />
            {row.required ? (
              <span className="field-required-star env-required-star" aria-hidden="true">
                *
              </span>
            ) : null}
          </div>
          <TextInput
            value={row.value}
            placeholder={t("profileEnvValue")}
            required={row.required}
            aria-required={row.required ? "true" : undefined}
            onInput={(event) => update(index, { value: event.currentTarget.value })}
          />
          <Tooltip content={t("profileEnvRemove")}>
            <span>
              <Button
                variant="ghost"
                className="env-remove-button"
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
      <Button className="secondary-button env-add-button" onClick={() => onChange([...items, { key: "", value: "" }])}>
        {t("profileEnvAdd")}
      </Button>
    </div>
  );
}

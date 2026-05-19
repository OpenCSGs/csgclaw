import { Button, TextInput } from "@/components/ui";
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
        <div key={index} className="env-row">
          <TextInput
            value={row.key}
            placeholder={t("profileEnvKey")}
            onInput={(event) => update(index, { key: event.currentTarget.value })}
          />
          <TextInput
            value={row.value}
            placeholder={t("profileEnvValue")}
            onInput={(event) => update(index, { value: event.currentTarget.value })}
          />
          <Button
            variant="ghost"
            className="env-remove-button"
            aria-label={t("profileEnvRemove")}
            title={t("profileEnvRemove")}
            onClick={() => remove(index)}
          >
            ×
          </Button>
        </div>
      ))}
      <Button className="secondary-button env-add-button" onClick={() => onChange([...items, { key: "", value: "" }])}>
        {t("profileEnvAdd")}
      </Button>
    </div>
  );
}

import type { FormEventHandler } from "react";
import { useId } from "react";
import { TextInput } from "@/components/ui";
import type { APIKeyProfile, Translator } from "./types";
import { requiredFieldLabel } from "./requiredFieldLabel";
import { isBlank } from "./utils";

export type APIKeyFieldProps = {
  label?: string;
  onInput?: FormEventHandler<HTMLInputElement>;
  profile?: APIKeyProfile | null;
  required?: boolean;
  t: Translator;
  value: string;
};

export function APIKeyField({ label, value, onInput, profile, required = false, t }: APIKeyFieldProps) {
  const generatedID = useId();
  const inputID = `${generatedID}-api-key`;
  const labelID = `${generatedID}-api-key-label`;
  const stored = Boolean(profile?.api_key_set);
  const preview = String(profile?.api_key_preview || "").trim();
  const showStoredMask = stored && isBlank(value);
  const previewPrefix = preview.endsWith("...") ? preview.slice(0, -3) : "";
  const placeholder = stored ? "" : t("profileAPIKeyNewPlaceholder");
  const labelText = label || t("profileAPIKey");
  return (
    <label className="field api-key-field" htmlFor={inputID}>
      {required ? (
        requiredFieldLabel(labelText, { id: labelID })
      ) : (
        <span id={labelID}>{labelText}</span>
      )}
      <div className="api-key-input-shell">
        <TextInput
          id={inputID}
          aria-labelledby={labelID}
          aria-required={required ? "true" : undefined}
          value={value}
          onInput={onInput}
          placeholder={placeholder}
          required={required}
          autoComplete="off"
          spellCheck={false}
        />
        {showStoredMask ? (
          <div className="api-key-mask" aria-hidden="true">
            {previewPrefix ? <span className="api-key-mask-prefix">{previewPrefix}</span> : null}
            <span className="api-key-mask-dots">••••••••</span>
          </div>
        ) : null}
      </div>
    </label>
  );
}

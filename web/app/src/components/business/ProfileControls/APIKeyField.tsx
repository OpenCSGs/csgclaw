import type { FormEventHandler } from "react";
import { useId } from "react";
import { TextInput } from "@/components/ui";
import type { APIKeyProfile, Translator } from "./types";
import { isBlank } from "./utils";

export type APIKeyFieldProps = {
  onInput?: FormEventHandler<HTMLInputElement>;
  profile?: APIKeyProfile | null;
  t: Translator;
  value: string;
};

export function APIKeyField({ value, onInput, profile, t }: APIKeyFieldProps) {
  const generatedID = useId();
  const inputID = `${generatedID}-api-key`;
  const labelID = `${generatedID}-api-key-label`;
  const stored = Boolean(profile?.api_key_set);
  const preview = String(profile?.api_key_preview || "").trim();
  const showStoredMask = stored && isBlank(value);
  const previewPrefix = preview.endsWith("...") ? preview.slice(0, -3) : "";
  const placeholder = stored ? "" : t("profileAPIKeyNewPlaceholder");
  return (
    <label className="field api-key-field" htmlFor={inputID}>
      <span id={labelID}>{t("profileAPIKey")}</span>
      <div className="api-key-input-shell">
        <TextInput
          id={inputID}
          aria-labelledby={labelID}
          value={value}
          onInput={onInput}
          placeholder={placeholder}
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

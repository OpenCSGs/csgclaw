import type { FocusEventHandler, FormEventHandler } from "react";
import { useState } from "react";
import { Eye, EyeOff } from "lucide-react";
import { Button, Field, TextInput } from "@/components/ui";
import type { APIKeyProfile, Translator } from "./types";
import { isBlank } from "./utils";

export type APIKeyFieldProps = {
  unchangedHint?: string;
  onBlur?: FocusEventHandler<HTMLInputElement>;
  onFocus?: FocusEventHandler<HTMLInputElement>;
  label?: string;
  onInput?: FormEventHandler<HTMLInputElement>;
  profile?: APIKeyProfile | null;
  required?: boolean;
  t: Translator;
  value: string;
};

export function APIKeyField({
  unchangedHint,
  label,
  value,
  onBlur,
  onFocus,
  onInput,
  profile,
  required = false,
  t,
}: APIKeyFieldProps) {
  const [focused, setFocused] = useState(false);
  const [visible, setVisible] = useState(false);
  const stored = Boolean(profile?.api_key_set);
  const preview = String(profile?.api_key_preview || "").trim();
  const showStoredMask = stored && !focused && isBlank(value);
  const placeholder = stored ? "" : t("profileAPIKeyNewPlaceholder");
  const labelText = label || t("profileAPIKey");
  return (
    <Field
      as="div"
      className="api-key-field"
      label={labelText}
      required={required}
      hint={stored ? unchangedHint : undefined}
    >
      <div className="api-key-input-shell">
        <TextInput
          type={visible ? "text" : "password"}
          value={value}
          onFocus={(event) => {
            setFocused(true);
            onFocus?.(event);
          }}
          onBlur={(event) => {
            setFocused(false);
            onBlur?.(event);
          }}
          onInput={onInput}
          placeholder={placeholder}
          required={required}
          autoComplete="off"
          spellCheck={false}
        />
        {showStoredMask ? (
          <div className="api-key-mask" aria-hidden="true">
            {preview || "••••••••"}
          </div>
        ) : null}
        <Button
          className="api-key-visibility-toggle"
          variant="tertiaryGray"
          size="sm"
          type="button"
          aria-label={t(visible ? "connectorsHideSecret" : "connectorsShowSecret")}
          aria-pressed={visible}
          onClick={() => setVisible((current) => !current)}
        >
          {visible ? <EyeOff size={16} aria-hidden="true" /> : <Eye size={16} aria-hidden="true" />}
        </Button>
      </div>
    </Field>
  );
}

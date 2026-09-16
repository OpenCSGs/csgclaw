import { useEffect, useId, useRef, useState } from "react";
import { X } from "lucide-react";
import {
  Button,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Tooltip,
} from "@/components/ui";
import {
  AUTH_ENVIRONMENT_PRESETS,
  authEnvironmentDraftFromPreset,
  authEnvironmentLoginReady,
} from "@/models/authEnvironment";
import type { AuthEnvironmentDraft, AuthEnvironmentPresetID } from "@/models/authEnvironment";
import type { TranslateFn } from "@/models/conversations";
import styles from "./OpenCSGConnectionDialog.module.css";

export type OpenCSGConnectionDialogProps = {
  busy: boolean;
  environment: AuthEnvironmentDraft;
  error?: string;
  open: boolean;
  t: TranslateFn;
  variant?: "connect" | "authentication-required";
  onConnect: (environment: AuthEnvironmentDraft) => void;
  onOpenChange: (open: boolean) => void;
};

export function OpenCSGConnectionDialog({
  busy,
  environment,
  error = "",
  open,
  t,
  variant = "connect",
  onConnect,
  onOpenChange,
}: OpenCSGConnectionDialogProps) {
  const customFieldErrorID = useId();
  const environmentFieldName = useId();
  const previousOpenRef = useRef(false);
  const [draft, setDraft] = useState(environment);
  const [customFieldTouched, setCustomFieldTouched] = useState(false);
  const ready = authEnvironmentLoginReady(draft);
  const showCustomFieldError = draft.preset === "custom" && customFieldTouched && !ready;
  const authenticationRequired = variant === "authentication-required";

  useEffect(() => {
    if (open && !previousOpenRef.current) {
      setDraft(environment);
      setCustomFieldTouched(false);
    }
    previousOpenRef.current = open;
  }, [environment, open]);

  function selectPreset(preset: AuthEnvironmentPresetID) {
    if (preset === "custom") {
      setCustomFieldTouched(false);
      setDraft({
        preset: "custom",
        opencsgBaseURL: draft.preset === "custom" ? draft.opencsgBaseURL : "",
        csgHubBaseURL: "",
        aiGatewayBaseURL: "",
      });
      return;
    }
    setCustomFieldTouched(false);
    setDraft(authEnvironmentDraftFromPreset(preset));
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen && busy) {
      return;
    }
    if (!nextOpen) {
      setCustomFieldTouched(false);
    }
    onOpenChange(nextOpen);
  }

  return (
    <DialogRoot open={open} onOpenChange={handleOpenChange}>
      <DialogContent className={styles.dialog}>
        <DialogHeader className={styles.header}>
          <div className={styles.copy}>
            <DialogTitle>{t(authenticationRequired ? "openCSGLoginRequiredTitle" : "csghubConnectTitle")}</DialogTitle>
            <DialogDescription>
              {t(authenticationRequired ? "openCSGLoginRequiredDescription" : "csghubConnectDescription")}
            </DialogDescription>
          </div>
          <OpenCSGDialogCloseButton disabled={busy} label={t("close")} />
        </DialogHeader>
        <DialogBody className={styles.body}>
          <fieldset className={styles.options}>
            <legend className={styles.srOnly}>{t("csghubLoginEnvironment")}</legend>
            {AUTH_ENVIRONMENT_PRESETS.map((preset) => (
              <EnvironmentOption
                key={preset.id}
                checked={draft.preset === preset.id}
                description={preset.label}
                name={environmentFieldName}
                label={preset.id === "prod" ? t("csghubEnvProduction") : t("csghubEnvStage")}
                value={preset.id}
                onChange={selectPreset}
              />
            ))}
            <EnvironmentOption
              checked={draft.preset === "custom"}
              description={t("csghubEnvCustomDescription")}
              name={environmentFieldName}
              label={t("csghubEnvCustom")}
              value="custom"
              onChange={selectPreset}
            />
          </fieldset>

          {draft.preset === "custom" ? (
            <label className={styles.customField}>
              <span>{t("csghubOpenCSGBaseURL")}</span>
              <input
                autoFocus
                aria-describedby={showCustomFieldError ? customFieldErrorID : undefined}
                aria-invalid={showCustomFieldError || undefined}
                value={draft.opencsgBaseURL}
                placeholder="https://openeast.opencsg.com"
                onChange={(event) => {
                  setCustomFieldTouched(true);
                  setDraft({
                    preset: "custom",
                    opencsgBaseURL: event.currentTarget.value,
                    csgHubBaseURL: "",
                    aiGatewayBaseURL: "",
                  });
                }}
              />
              {showCustomFieldError ? (
                <span id={customFieldErrorID} className={styles.fieldError}>
                  {t("csghubInvalidSiteURL")}
                </span>
              ) : null}
            </label>
          ) : null}

          <p className={styles.returnHint}>
            {t(authenticationRequired ? "openCSGLoginRequiredHint" : "csghubConnectReturnHint")}
          </p>
          {error ? <div className="form-error">{error}</div> : null}
        </DialogBody>
        <DialogFooter className={styles.actions}>
          <Button variant="secondaryGray" size="md" disabled={busy} onClick={() => handleOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button variant="primary" size="md" loading={busy} disabled={!ready} onClick={() => onConnect(draft)}>
            {t(authenticationRequired ? "csghubSignIn" : "csghubConnectContinue")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

type OpenCSGSwitchDialogProps = {
  accountName: string;
  busy: boolean;
  environmentLabel: string;
  open: boolean;
  t: TranslateFn;
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
};

export function OpenCSGSwitchDialog({
  accountName,
  busy,
  environmentLabel,
  open,
  t,
  onConfirm,
  onOpenChange,
}: OpenCSGSwitchDialogProps) {
  return (
    <DialogRoot open={open} onOpenChange={onOpenChange}>
      <DialogContent className={styles.dialog}>
        <DialogHeader className={styles.header}>
          <div className={styles.copy}>
            <DialogTitle>{t("csghubSwitchConfirmTitle")}</DialogTitle>
            <DialogDescription>
              {t("csghubSwitchConfirmDescription", { environment: environmentLabel, user: accountName })}
            </DialogDescription>
          </div>
          <OpenCSGDialogCloseButton label={t("close")} />
        </DialogHeader>
        <DialogBody>
          <p className={styles.switchNote}>{t("csghubSwitchConfirmNote")}</p>
        </DialogBody>
        <DialogFooter className={styles.actions}>
          <Button variant="secondaryGray" size="md" disabled={busy} onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button variant="primary" size="md" loading={busy} onClick={onConfirm}>
            {t("csghubSwitchConfirmAction")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

type EnvironmentOptionProps = {
  checked: boolean;
  description: string;
  label: string;
  name: string;
  value: AuthEnvironmentPresetID;
  onChange: (value: AuthEnvironmentPresetID) => void;
};

function EnvironmentOption({ checked, description, label, name, value, onChange }: EnvironmentOptionProps) {
  return (
    <label className={styles.option}>
      <input checked={checked} name={name} type="radio" value={value} onChange={() => onChange(value)} />
      <span>
        <strong>{label}</strong>
        <small>{description}</small>
      </span>
    </label>
  );
}

function OpenCSGDialogCloseButton({ disabled = false, label }: { disabled?: boolean; label: string }) {
  return (
    <Tooltip content={label}>
      <DialogClose asChild>
        <button type="button" className={styles.closeButton} aria-label={label} disabled={disabled}>
          <X size={18} strokeWidth={1.75} aria-hidden="true" />
        </button>
      </DialogClose>
    </Tooltip>
  );
}

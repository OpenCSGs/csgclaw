import { useId, useState } from "react";
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

type OpenCSGConnectionDialogProps = {
  busy: boolean;
  draft: AuthEnvironmentDraft;
  open: boolean;
  t: TranslateFn;
  onConnect: () => void;
  onDraftChange: (draft: AuthEnvironmentDraft) => void;
  onOpenChange: (open: boolean) => void;
};

export function OpenCSGConnectionDialog({
  busy,
  draft,
  open,
  t,
  onConnect,
  onDraftChange,
  onOpenChange,
}: OpenCSGConnectionDialogProps) {
  const customFieldErrorID = useId();
  const [customFieldTouched, setCustomFieldTouched] = useState(false);
  const ready = authEnvironmentLoginReady(draft);
  const showCustomFieldError = draft.preset === "custom" && customFieldTouched && !ready;

  function selectPreset(preset: AuthEnvironmentPresetID) {
    if (preset === "custom") {
      setCustomFieldTouched(false);
      onDraftChange({
        preset: "custom",
        opencsgBaseURL: draft.preset === "custom" ? draft.opencsgBaseURL : "",
        csgHubBaseURL: "",
        aiGatewayBaseURL: "",
      });
      return;
    }
    setCustomFieldTouched(false);
    onDraftChange(authEnvironmentDraftFromPreset(preset));
  }

  function handleOpenChange(nextOpen: boolean) {
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
            <DialogTitle>{t("csghubConnectTitle")}</DialogTitle>
            <DialogDescription>{t("csghubConnectDescription")}</DialogDescription>
          </div>
          <OpenCSGDialogCloseButton label={t("close")} />
        </DialogHeader>
        <DialogBody className={styles.body}>
          <fieldset className={styles.options}>
            <legend className={styles.srOnly}>{t("csghubLoginEnvironment")}</legend>
            {AUTH_ENVIRONMENT_PRESETS.map((preset) => (
              <EnvironmentOption
                key={preset.id}
                checked={draft.preset === preset.id}
                description={preset.label}
                label={preset.id === "prod" ? t("csghubEnvProduction") : t("csghubEnvStage")}
                value={preset.id}
                onChange={selectPreset}
              />
            ))}
            <EnvironmentOption
              checked={draft.preset === "custom"}
              description={t("csghubEnvCustomDescription")}
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
                  onDraftChange({
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

          <p className={styles.returnHint}>{t("csghubConnectReturnHint")}</p>
        </DialogBody>
        <DialogFooter className={styles.actions}>
          <Button variant="secondaryGray" size="md" disabled={busy} onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button variant="primary" size="md" loading={busy} disabled={!ready} onClick={onConnect}>
            {t("csghubConnectContinue")}
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
  value: AuthEnvironmentPresetID;
  onChange: (value: AuthEnvironmentPresetID) => void;
};

function EnvironmentOption({ checked, description, label, value, onChange }: EnvironmentOptionProps) {
  return (
    <label className={styles.option}>
      <input checked={checked} name="opencsg-environment" type="radio" value={value} onChange={() => onChange(value)} />
      <span>
        <strong>{label}</strong>
        <small>{description}</small>
      </span>
    </label>
  );
}

function OpenCSGDialogCloseButton({ label }: { label: string }) {
  return (
    <Tooltip content={label}>
      <DialogClose asChild>
        <button type="button" className={styles.closeButton} aria-label={label}>
          <X size={18} strokeWidth={1.75} aria-hidden="true" />
        </button>
      </DialogClose>
    </Tooltip>
  );
}

import { fetchAuthLoginSites } from "@/api/auth";
import type { AuthLoginSite } from "@/api/auth";
import { useEffect, useId, useRef, useState } from "react";
import {
  InlineError,
  Select,
  TextInput,
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
} from "@/components/ui";
import {
  AUTH_ENVIRONMENT_PRESETS,
  authEnvironmentDraftFromPreset,
  authEnvironmentLoginReady,
} from "@/models/authEnvironment";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import styles from "./OpenCSGConnectionDialog.module.css";

const CUSTOM_SITE = "custom";

function customSiteDraft(url: string): AuthEnvironmentDraft {
  return { preset: "custom", opencsgBaseURL: url, csgHubBaseURL: "", aiGatewayBaseURL: "" };
}

type SiteCatalog = { status: "loading" | "unavailable" } | { status: "ready"; sites: AuthLoginSite[] };

export type OpenCSGConnectionDialogProps = {
  busy: boolean;
  environment: AuthEnvironmentDraft;
  error?: string;
  locale: LocaleCode;
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
  locale,
  open,
  t,
  variant = "connect",
  onConnect,
  onOpenChange,
}: OpenCSGConnectionDialogProps) {
  const customFieldErrorID = useId();
  const siteFieldID = useId();
  const previousOpenRef = useRef(false);
  const [draft, setDraft] = useState(environment);
  const [customFieldTouched, setCustomFieldTouched] = useState(false);
  const [manual, setManual] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [catalog, setCatalog] = useState<SiteCatalog>({ status: "loading" });
  const siteLocale = locale === "zh" ? "zh" : "en";
  const sites = catalog.status === "ready" ? catalog.sites : [];
  const selectedSite = manual ? undefined : sites.find((site) => site.url === draft.opencsgBaseURL);
  const showCustomField = manual || (!selectedSite && catalog.status !== "loading");
  const ready = Boolean(selectedSite || showCustomField) && authEnvironmentLoginReady(draft);
  const showCustomFieldError = showCustomField && customFieldTouched && !ready;
  const authenticationRequired = variant === "authentication-required";

  useEffect(() => {
    if (open && !previousOpenRef.current) {
      setDraft(environment);
      setManual(false);
      setCustomFieldTouched(false);
    }
    previousOpenRef.current = open;
  }, [environment, open]);

  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    setCatalog({ status: "loading" });
    void fetchAuthLoginSites(controller.signal).then(
      (sites) => {
        if (!controller.signal.aborted) setCatalog({ status: "ready", sites });
      },
      () => {
        if (!controller.signal.aborted) setCatalog({ status: "unavailable" });
      },
    );
    return () => controller.abort();
  }, [attempt, open]);

  function selectSite(value: string) {
    setCustomFieldTouched(false);
    if (value === CUSTOM_SITE) {
      setManual(true);
      setDraft(customSiteDraft(selectedSite ? "" : draft.opencsgBaseURL));
      return;
    }
    const site = sites.find((item) => item.id === value);
    if (!site) return;
    setManual(false);
    const preset = AUTH_ENVIRONMENT_PRESETS.find((item) => item.opencsgBaseURL === site.url);
    setDraft(preset ? authEnvironmentDraftFromPreset(preset.id) : customSiteDraft(site.url));
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
          <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly disabled={busy} />
        </DialogHeader>
        <DialogBody className={styles.body}>
          <div className={styles.customField}>
            <label htmlFor={siteFieldID}>{t("csghubLoginEnvironment")}</label>
            <Select
              disabled={busy}
              value={selectedSite?.id ?? (showCustomField ? CUSTOM_SITE : "")}
              onValueChange={selectSite}
              triggerProps={{ id: siteFieldID }}
              triggerClassName={styles.siteTrigger}
              placeholder={t("csghubLoginSitesLoading")}
              selectedLabel={
                selectedSite ? (
                  <span className={styles.siteOption}>
                    <img src={selectedSite.icon} alt="" />
                    <span className={styles.siteCopy}>
                      <span>{selectedSite.locales[siteLocale].title}</span>
                      <small>{new URL(selectedSite.url).host}</small>
                    </span>
                  </span>
                ) : undefined
              }
              options={[
                ...sites.map((site, index) => ({
                  value: site.id,
                  textValue: site.locales[siteLocale].title,
                  label: (
                    <span className={styles.siteOption}>
                      <img src={site.icon} alt="" />
                      {site.locales[siteLocale].title}
                    </span>
                  ),
                  description: new URL(site.url).host,
                  separatorAfter: index === sites.length - 1,
                })),
                { value: CUSTOM_SITE, label: t("csghubEnvCustom") },
              ]}
            />
            <span className="sr-only" role="status">
              {catalog.status === "loading" ? t("csghubLoginSitesLoading") : ""}
            </span>
            {catalog.status === "unavailable" ? (
              <InlineError>
                {t("csghubLoginSitesUnavailable")}{" "}
                <Button variant="tertiaryGray" size="sm" disabled={busy} onClick={() => setAttempt(attempt + 1)}>
                  {t("retry")}
                </Button>
              </InlineError>
            ) : null}
          </div>

          {showCustomField ? (
            <label className={styles.customField}>
              <span>{t("csghubOpenCSGBaseURL")}</span>
              <TextInput
                disabled={busy}
                aria-describedby={showCustomFieldError ? customFieldErrorID : undefined}
                aria-invalid={showCustomFieldError || undefined}
                value={draft.opencsgBaseURL}
                placeholder="https://openeast.opencsg.com"
                onChange={(event) => {
                  setCustomFieldTouched(true);
                  setManual(true);
                  setDraft(customSiteDraft(event.currentTarget.value));
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
          {error ? <InlineError>{error}</InlineError> : null}
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
          <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly />
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

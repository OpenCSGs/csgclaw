import { CircleCheck, CircleX } from "lucide-react";
import { checkAuthSite, fetchAuthLoginSites } from "@/api/auth";
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
  normalizeBaseURL,
} from "@/models/authEnvironment";
import type { AuthEnvironmentDraft } from "@/models/authEnvironment";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import { readStoredAuthCustomSites, writeStoredAuthCustomSites } from "@/shared/storage/authEnvironment";
import type { SavedAuthSite } from "@/shared/storage/authEnvironment";
import styles from "./OpenCSGConnectionDialog.module.css";

const CUSTOM_SITE = "custom";
const SAVED_SITE_PREFIX = "saved:";
const CATALOG_SITE_PREFIX = "catalog:";

function customSiteDraft(url: string): AuthEnvironmentDraft {
  return { preset: "custom", opencsgBaseURL: url, csgHubBaseURL: "", aiGatewayBaseURL: "" };
}

function normalizeSiteInput(raw: string): string {
  const address = raw.trim();
  const bareHost = /^(?:localhost|\[[a-f0-9:]+\]|[^\s/:@]+\.[^\s/:@]+)(?::\d+)?(?:[/?#]|$)/i;
  return normalizeBaseURL(bareHost.test(address) ? `https://${address}` : address);
}

function siteDraft(url: string): AuthEnvironmentDraft {
  const preset = AUTH_ENVIRONMENT_PRESETS.find((item) => item.opencsgBaseURL === url);
  return preset ? authEnvironmentDraftFromPreset(preset.id) : customSiteDraft(url);
}

type SiteCatalog = { status: "loading" | "unavailable" } | { status: "ready"; sites: AuthLoginSite[] };

export type OpenCSGConnectionDialogProps = {
  busy: boolean;
  environment: AuthEnvironmentDraft;
  error?: string;
  locale: LocaleCode;
  open: boolean;
  t: TranslateFn;
  variant?: "connect" | "authentication-required" | "switch-site";
  onCloseAutoFocus?: (event: Event) => void;
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
  onCloseAutoFocus,
  onConnect,
  onOpenChange,
}: OpenCSGConnectionDialogProps) {
  const customFieldErrorID = useId();
  const siteFieldID = useId();
  const customFieldHintID = useId();
  const previousOpenRef = useRef(false);
  const [draft, setDraft] = useState(environment);
  const [customFieldTouched, setCustomFieldTouched] = useState(false);
  const [manual, setManual] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [catalog, setCatalog] = useState<SiteCatalog>({ status: "loading" });
  const [savedSites, setSavedSites] = useState(readStoredAuthCustomSites);
  const [saveError, setSaveError] = useState(false);
  const [siteName, setSiteName] = useState("");
  const [editingURL, setEditingURL] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [probe, setProbe] = useState<"idle" | "testing" | "failed" | number>("idle");
  const probeRequest = useRef<AbortController | null>(null);
  const siteLocale = locale === "zh" ? "zh" : "en";
  const sites = catalog.status === "ready" ? catalog.sites : [];
  const draftURL = manual ? normalizeSiteInput(draft.opencsgBaseURL) : normalizeBaseURL(draft.opencsgBaseURL);
  const selectedSite = manual ? undefined : sites.find((site) => site.url === draftURL);
  const localSites = savedSites.filter((saved) => !sites.some((site) => site.url === saved.url));
  const selectedSavedSite = manual ? undefined : savedSites.find((saved) => saved.url === draftURL);
  const showCustomField = manual || (!selectedSite && !selectedSavedSite && catalog.status !== "loading");
  const ready =
    Boolean(selectedSite || selectedSavedSite || showCustomField) &&
    authEnvironmentLoginReady(customSiteDraft(draftURL));
  const showCustomFieldError = showCustomField && customFieldTouched && !ready;
  const duplicateEdit = Boolean(
    editingURL && editingURL !== draftURL && savedSites.some((saved) => saved.url === draftURL),
  );
  const switching = variant === "switch-site";
  const sameSite = switching && draftURL === normalizeBaseURL(environment.opencsgBaseURL);
  const authenticationRequired = variant === "authentication-required";
  const targetLabel =
    selectedSite?.locales[siteLocale].title || selectedSavedSite?.name || (ready ? new URL(draftURL).host : "");

  useEffect(() => {
    if (open && !previousOpenRef.current) {
      setDraft(environment);
      setSavedSites(readStoredAuthCustomSites());
      setSaveError(false);
      setManual(false);
      setSiteName("");
      setEditingURL("");
      setConfirmDelete(false);
      setProbe("idle");
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
        if (!controller.signal.aborted) {
          setCatalog({ status: "ready", sites: sites.map((site) => ({ ...site, url: normalizeBaseURL(site.url) })) });
        }
      },
      () => {
        if (!controller.signal.aborted) setCatalog({ status: "unavailable" });
      },
    );
    return () => controller.abort();
  }, [attempt, open]);

  useEffect(() => {
    return () => probeRequest.current?.abort();
  }, [open]);

  function resetProbe() {
    probeRequest.current?.abort();
    setProbe("idle");
  }

  async function testSite() {
    resetProbe();
    const controller = new AbortController();
    probeRequest.current = controller;
    setProbe("testing");
    try {
      const result = await checkAuthSite(draftURL, controller.signal);
      if (!controller.signal.aborted) setProbe(result.latency_ms);
    } catch (_) {
      if (!controller.signal.aborted) setProbe("failed");
    }
  }

  function selectSite(value: string) {
    resetProbe();
    setSiteName("");
    setEditingURL("");
    setConfirmDelete(false);
    setCustomFieldTouched(false);
    setSaveError(false);
    if (value === CUSTOM_SITE) {
      setManual(true);
      setDraft(customSiteDraft(selectedSite || selectedSavedSite ? "" : draft.opencsgBaseURL));
      return;
    }
    const savedSite = localSites.find((saved) => SAVED_SITE_PREFIX + saved.url === value);
    if (savedSite) {
      setManual(false);
      setDraft(siteDraft(savedSite.url));
      return;
    }
    const site = sites.find((item) => CATALOG_SITE_PREFIX + item.id === value);
    if (!site) return;
    setManual(false);
    setDraft(siteDraft(site.url));
  }

  function persistSites(next: SavedAuthSite[]): boolean {
    try {
      writeStoredAuthCustomSites(next);
      setSavedSites(next);
      setSaveError(false);
      return true;
    } catch (_) {
      setSaveError(true);
      return false;
    }
  }

  function saveCustomSite() {
    if (!ready) return;
    const current = readStoredAuthCustomSites();
    const existing = current.find((saved) => saved.url === draftURL);
    if (editingURL && editingURL !== draftURL && existing) {
      setSavedSites(current);
      return;
    }
    const saved = { url: draftURL, name: siteName.trim() || (editingURL ? "" : existing?.name) || "" };
    const replacedURL = editingURL || draftURL;
    const next = current.some((item) => item.url === replacedURL)
      ? current.map((item) => (item.url === replacedURL ? saved : item))
      : [...current, saved];
    if (!persistSites(next)) return;
    setDraft(siteDraft(draftURL));
    setManual(false);
    setEditingURL("");
    document.getElementById(siteFieldID)?.focus();
  }

  function deleteSite(url: string) {
    if (!persistSites(readStoredAuthCustomSites().filter((saved) => saved.url !== url))) return;
    setConfirmDelete(false);
    resetProbe();
    setDraft(authEnvironmentDraftFromPreset("prod"));
    document.getElementById(siteFieldID)?.focus();
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
      <DialogContent className={styles.dialog} onCloseAutoFocus={onCloseAutoFocus}>
        <DialogHeader className={styles.header}>
          <div className={styles.copy}>
            <DialogTitle>
              {t(
                editingURL
                  ? "csghubSiteEdit"
                  : authenticationRequired
                    ? "openCSGLoginRequiredTitle"
                    : "csghubConnectTitle",
              )}
            </DialogTitle>
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
              value={
                selectedSite
                  ? CATALOG_SITE_PREFIX + selectedSite.id
                  : selectedSavedSite
                    ? SAVED_SITE_PREFIX + selectedSavedSite.url
                    : showCustomField
                      ? CUSTOM_SITE
                      : ""
              }
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
                ) : selectedSavedSite ? (
                  <span className={styles.siteOption}>
                    <span className={styles.siteCopy}>
                      <span>{selectedSavedSite.name || selectedSavedSite.url}</span>
                      {selectedSavedSite.name ? <small>{selectedSavedSite.url}</small> : null}
                    </span>
                  </span>
                ) : undefined
              }
              options={[
                ...sites.map((site, index) => ({
                  value: CATALOG_SITE_PREFIX + site.id,
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
                ...localSites.map(({ url, name }) => ({
                  value: SAVED_SITE_PREFIX + url,
                  textValue: name ? `${name} ${url}` : url,
                  label: <span className={styles.siteOption}>{name || url}</span>,
                  description: name ? url : undefined,
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
            <div className={styles.customField}>
              <label className={styles.customField}>
                {t("csghubSiteName")}
                <TextInput
                  disabled={busy}
                  value={siteName}
                  maxLength={40}
                  onChange={(event) => setSiteName(event.currentTarget.value)}
                />
              </label>
              <label className={styles.customField}>
                <span>{t("csghubOpenCSGBaseURL")}</span>
                <TextInput
                  disabled={busy}
                  aria-describedby={[customFieldHintID, showCustomFieldError ? customFieldErrorID : ""]
                    .filter(Boolean)
                    .join(" ")}
                  aria-invalid={showCustomFieldError || undefined}
                  value={draft.opencsgBaseURL}
                  placeholder="https://openeast.opencsg.com"
                  onChange={(event) => {
                    resetProbe();
                    setCustomFieldTouched(true);
                    setSaveError(false);
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
              <p id={customFieldHintID} className={styles.returnHint}>
                {t(
                  !ready
                    ? "csghubCustomSiteInputHint"
                    : editingURL
                      ? "csghubSiteEditingHint"
                      : "csghubCustomSiteSaveHint",
                )}
              </p>
              {duplicateEdit ? <InlineError>{t("csghubSiteDuplicate")}</InlineError> : null}
            </div>
          ) : null}

          <div className={styles.siteActions}>
            {showCustomField && !editingURL ? (
              <>
                <Button
                  variant="secondaryGray"
                  size="sm"
                  disabled={busy || !ready || duplicateEdit}
                  onClick={saveCustomSite}
                >
                  {t("csghubSaveCustomSite")}
                </Button>
              </>
            ) : null}
            <Button
              variant="secondaryGray"
              size="sm"
              disabled={busy || !ready}
              loading={probe === "testing"}
              loadingLabel={t("csghubSiteTesting")}
              onClick={() => void testSite()}
            >
              {t("csghubSiteTest")}
            </Button>
            {selectedSavedSite && !showCustomField ? (
              <div className={styles.siteManagement}>
                <Button
                  variant="secondaryGray"
                  size="sm"
                  disabled={busy || confirmDelete}
                  onClick={() => {
                    setSiteName(selectedSavedSite.name);
                    setEditingURL(selectedSavedSite.url);
                    setManual(true);
                  }}
                >
                  {t("csghubSiteEdit")}
                </Button>
                {confirmDelete ? (
                  <>
                    <Button
                      variant="danger"
                      size="sm"
                      disabled={busy}
                      onClick={() => deleteSite(selectedSavedSite.url)}
                    >
                      {t("csghubSiteConfirmDelete")}
                    </Button>
                    <Button variant="tertiaryGray" size="sm" disabled={busy} onClick={() => setConfirmDelete(false)}>
                      {t("cancel")}
                    </Button>
                  </>
                ) : (
                  <Button variant="outlineDanger" size="sm" disabled={busy} onClick={() => setConfirmDelete(true)}>
                    {t("delete")}
                  </Button>
                )}
              </div>
            ) : null}
          </div>
          {sameSite ? <p className={styles.returnHint}>{t("csghubSiteAlreadyConnected")}</p> : null}
          {typeof probe === "number" ? (
            <p role="status" className={`${styles.probeResult} ${styles.probeSuccess}`}>
              <CircleCheck size={16} aria-hidden="true" />
              <span>{t("csghubSiteTestPassed", { latency: String(probe) })}</span>
            </p>
          ) : null}
          {probe === "failed" ? (
            <p role="alert" className={`${styles.probeResult} ${styles.probeFailure}`}>
              <CircleX size={16} aria-hidden="true" />
              <span>{t("csghubSiteTestFailed")}</span>
            </p>
          ) : null}
          {saveError ? <InlineError>{t("csghubSaveCustomSiteFailed")}</InlineError> : null}

          {!editingURL ? (
            <p className={styles.returnHint}>
              {ready && !sameSite ? t("csghubLoginTargetHint", { site: targetLabel }) : null}
              {ready && !sameSite ? " " : null}
              {t(authenticationRequired ? "openCSGLoginRequiredHint" : "csghubConnectReturnHint")}
            </p>
          ) : null}
          {error ? <InlineError>{error}</InlineError> : null}
        </DialogBody>
        <DialogFooter className={styles.actions}>
          {editingURL ? (
            <>
              <Button
                variant="secondaryGray"
                size="md"
                disabled={busy}
                onClick={() => {
                  setDraft(siteDraft(editingURL));
                  setManual(false);
                  setEditingURL("");
                  setSaveError(false);
                  resetProbe();
                }}
              >
                {t("csghubSiteCancelEdit")}
              </Button>
              <Button variant="primary" size="md" disabled={busy || !ready || duplicateEdit} onClick={saveCustomSite}>
                {t("csghubSiteSaveChanges")}
              </Button>
            </>
          ) : (
            <>
              <Button variant="secondaryGray" size="md" disabled={busy} onClick={() => handleOpenChange(false)}>
                {t("cancel")}
              </Button>
              <Button
                variant="primary"
                size="md"
                loading={busy}
                disabled={!ready || sameSite || Boolean(editingURL)}
                onClick={() => onConnect(manual ? siteDraft(draftURL) : draft)}
              >
                {t(
                  switching
                    ? "csghubSwitchEnvironment"
                    : authenticationRequired
                      ? "csghubSignIn"
                      : "csghubConnectContinue",
                )}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

type OpenCSGSwitchDialogProps = {
  accountName: string;
  busy: boolean;
  environmentLabel: string;
  targetLabel: string;
  error?: string;
  open: boolean;
  t: TranslateFn;
  onConfirm: () => void;
  onOpenChange: (open: boolean) => void;
};

export function OpenCSGSwitchDialog({
  accountName,
  busy,
  environmentLabel,
  targetLabel,
  error,
  open,
  t,
  onConfirm,
  onOpenChange,
}: OpenCSGSwitchDialogProps) {
  return (
    <DialogRoot
      open={open}
      onOpenChange={(next) => {
        if (!busy) onOpenChange(next);
      }}
    >
      <DialogContent className={styles.dialog}>
        <DialogHeader className={styles.header}>
          <div className={styles.copy}>
            <DialogTitle>{t("csghubSwitchConfirmTitle")}</DialogTitle>
            <DialogDescription>
              {t("csghubSwitchConfirmDescription", {
                environment: environmentLabel,
                user: accountName,
                target: targetLabel,
              })}
            </DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} size="sm" variant="tertiaryGray" iconOnly disabled={busy} />
        </DialogHeader>
        <DialogBody>
          <p className={styles.switchNote}>{t("csghubSwitchConfirmNote")}</p>
          {error ? <InlineError>{error}</InlineError> : null}
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

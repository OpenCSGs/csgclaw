import { useEffect, useRef, useState } from "react";
import type { AppConfig, AppDefinition, AppInstallation, AppProbeResult } from "@/api/apps";
import { errorMessage } from "@/api/client";
import {
  Button,
  DialogBody,
  DialogCloseButton,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogRoot,
  DialogTitle,
  Field,
  Select,
  TextArea,
  TextInput,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIError } from "@/shared/i18n";
import {
  appFormPayload,
  defaultPlatformCredentialSource,
  appName,
  initialAppForm,
  type AppForm,
  type AppValueRow,
} from "./appForm";
import { AppToolList } from "./AppToolList";
import { AppKnowledgePicker } from "./AppKnowledgePicker";
import styles from "./AgentAppsPanel.module.css";

type Props = {
  definition: AppDefinition;
  globalResource?: boolean;
  existing: AppInstallation | null;
  t: TranslateFn;
  portalContainer?: HTMLElement | null;
  onClose: () => void;
  onProbe: (payload: ReturnType<typeof appFormPayload>) => Promise<AppProbeResult>;
  onSave: (payload: ReturnType<typeof appFormPayload>, connect: boolean) => Promise<unknown>;
};

export function AppSettingsDialog({
  definition,
  globalResource = false,
  existing,
  t,
  portalContainer,
  onClose,
  onProbe,
  onSave,
}: Props) {
  const [form, setForm] = useState<AppForm>(() => initialAppForm(definition, existing));
  const [busy, setBusy] = useState<"probe" | "save" | "connect" | "">("");
  const [error, setError] = useState("");
  const [probe, setProbe] = useState<{ signature: string; result: AppProbeResult } | null>(null);
  const signature = JSON.stringify(form);
  const [initialSignature] = useState(signature);
  const tested = probe?.signature === signature && probe.result.connected;
  useEffect(() => {
    setError("");
  }, [signature]);
  const probeResultRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (tested) probeResultRef.current?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [tested]);
  const config = form.config;
  const stdio = config.transport === "stdio";
  const feishu = definition.app_id === "feishu";
  const gitlab = definition.app_id === "gitlab";
  const platformSource = config.platform_credential_source || defaultPlatformCredentialSource(config);
  const appCredentials = config.auth_mode === "feishu";
  const oauthUnsupported = config.auth_mode === "oauth2" && !definition.oauth_supported;
  const hasSavedCredential = (key: string) =>
    Boolean(existing?.credentials_set[key]) &&
    !(
      gitlab &&
      key === "token" &&
      config.gitlab_base_url?.trim().replace(/\/+$/, "") !==
        existing?.config.gitlab_base_url?.trim().replace(/\/+$/, "")
    );
  const hasCredential = (key: "token" | "app_id" | "app_secret") =>
    key === "app_id" || form.credentials[key] ? Boolean(form.credentials[key]?.trim()) : hasSavedCredential(key);
  const tokenRequired =
    gitlab ||
    (!appCredentials &&
      config.auth_mode !== "none" &&
      config.auth_mode !== "oauth2" &&
      (config.auth_mode === "header" || platformSource !== "opencsg_login"));
  const requiredFieldsFilled =
    Boolean(form.name.trim() && (stdio ? config.command?.trim() : config.url?.trim())) &&
    (!gitlab || Boolean(config.gitlab_base_url?.trim())) &&
    (!tokenRequired || hasCredential("token")) &&
    (!appCredentials || (hasCredential("app_id") && hasCredential("app_secret")));
  const credentialPlaceholder = (key: string) => (hasSavedCredential(key) ? "••••••••" : "");
  const updateConfig = (patch: Partial<AppConfig>) =>
    setForm((previous) => ({ ...previous, config: { ...previous.config, ...patch } }));

  async function execute(action: "probe" | "save" | "connect") {
    if (!requiredFieldsFilled || oauthUnsupported) return;
    setBusy(action);
    setError("");
    try {
      const payload = appFormPayload(form);
      if (action === "probe") {
        const result = await onProbe(payload);
        setProbe({ signature, result });
      } else {
        await onSave(payload, action === "connect");
        onClose();
      }
    } catch (failure) {
      setError(localizeAPIError(failure, t) || errorMessage(failure, t("appActionFailed")));
      setProbe(null);
    } finally {
      setBusy("");
    }
  }

  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent className={styles.settingsDialog} portalContainer={portalContainer}>
        <DialogHeader>
          <div>
            <DialogTitle>
              {existing
                ? t("appSettingsTitle", { name: existing.name })
                : t("appAddTitle", { name: appName(definition.app_id, t, definition.name) })}
            </DialogTitle>
            <DialogDescription>
              {t(globalResource ? "appSaveValidationHint" : "appSettingsDescription")}
            </DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} disabled={Boolean(busy)} size="sm" variant="tertiaryGray" />
        </DialogHeader>
        <DialogBody className={styles.settingsBody}>
          <fieldset className={styles.fields} disabled={Boolean(busy)}>
            <section className={styles.formSection} aria-label={t("appConnectionSection")}>
              <header className={styles.sectionHeading}>
                <h3>{t("appConnectionSection")}</h3>
                <p>{t("appConnectionSectionHint")}</p>
              </header>
              <div className={styles.columns}>
                <Field required label={t("appInstanceName")}>
                  <TextInput
                    required
                    aria-label={t("appInstanceName")}
                    autoComplete="off"
                    value={form.name}
                    onChange={(event) => setForm({ ...form, name: event.target.value })}
                  />
                </Field>
                {!gitlab ? (
                  <Field label={t("appTransport")}>
                    <Select
                      value={config.transport || "http"}
                      triggerProps={{ "aria-label": t("appTransport") }}
                      options={[
                        { value: "http", label: t("appTransportHTTP") },
                        { value: "stdio", label: t("appTransportStdio") },
                      ]}
                      onValueChange={(value) =>
                        updateConfig({
                          transport: value === "stdio" ? "stdio" : "http",
                          platform_credential_source: value === "stdio" ? "manual" : undefined,
                          auth_mode:
                            config.auth_mode === "feishu" ||
                            config.auth_mode === "none" ||
                            config.auth_mode === "oauth2"
                              ? config.auth_mode
                              : value === "stdio"
                                ? "env"
                                : "bearer",
                        })
                      }
                    />
                  </Field>
                ) : null}
              </div>
              {stdio ? (
                <>
                  <Field required label={t("appCommand")} hint={t("appCommandHint")}>
                    <TextInput
                      aria-label={t("appCommand")}
                      required
                      value={config.command || ""}
                      onChange={(event) => updateConfig({ command: event.target.value })}
                      placeholder="npx"
                    />
                  </Field>
                  <Field label={t("appArguments")} hint={t("appArgumentsHint")}>
                    <TextArea
                      aria-label={t("appArguments")}
                      rows={3}
                      value={form.args}
                      onChange={(event) => setForm({ ...form, args: event.target.value })}
                    />
                  </Field>
                  <Field label={t("appWorkingDirectory")}>
                    <TextInput
                      value={config.cwd || ""}
                      onChange={(event) => updateConfig({ cwd: event.target.value })}
                    />
                  </Field>
                </>
              ) : (
                <Field required label={t("appServiceURL")} hint={t("appServiceURLHint")}>
                  <TextInput
                    aria-label={t("appServiceURL")}
                    required
                    type="url"
                    value={config.url || ""}
                    onChange={(event) => updateConfig({ url: event.target.value })}
                    placeholder={gitlab ? "https://service.public.opencsg.com/mcp" : "https://example.com/mcp"}
                  />
                </Field>
              )}
              {gitlab ? (
                <Field required label={t("appGitLabInstanceURL")} hint={t("appGitLabInstanceURLHint")}>
                  <TextInput
                    aria-label={t("appGitLabInstanceURL")}
                    required
                    type="url"
                    value={config.gitlab_base_url || ""}
                    onChange={(event) => updateConfig({ gitlab_base_url: event.target.value })}
                    placeholder="https://gitlab.example.com"
                  />
                </Field>
              ) : null}
              {!stdio && definition.app_id === "llm-wiki" ? (
                <AppKnowledgePicker t={t} onSelect={(url) => updateConfig({ url, transport: "http" })} />
              ) : null}
            </section>
            <section className={styles.formSection} aria-label={t("appAccessSection")}>
              <header className={styles.sectionHeading}>
                <h3>{t("appAccessSection")}</h3>
                <p>{t("appAccessSectionHint")}</p>
              </header>
              <div className={styles.columns}>
                {!gitlab ? (
                  <Field label={t("appAuthentication")}>
                    <Select
                      value={config.auth_mode || "none"}
                      triggerProps={{ "aria-label": t("appAuthentication") }}
                      options={[
                        ...(!stdio
                          ? [
                              { value: "bearer", label: t("appAuthBearer") },
                              { value: "header", label: t("appAuthHeader") },
                            ]
                          : []),
                        ...(feishu ? [{ value: "feishu", label: t("appAuthFeishu") }] : []),
                        ...(stdio ? [{ value: "env", label: t("appAuthEnvironment") }] : []),
                        { value: "none", label: t("appAuthNone") },
                        ...(!stdio ? [{ value: "oauth2", label: t("appAuthOAuth") }] : []),
                      ]}
                      onValueChange={(value) =>
                        updateConfig({
                          auth_mode: value as AppConfig["auth_mode"],
                          platform_credential_source:
                            value === "env" || value === "oauth2" ? "manual" : config.platform_credential_source,
                          credential_source: "manual",
                        })
                      }
                    />
                  </Field>
                ) : null}
                {!stdio && config.auth_mode !== "oauth2" ? (
                  <Field
                    label={t("appPlatformCredentialSource")}
                    hint={platformSource === "opencsg_login" ? t("appOpenCSGLoginHint") : undefined}
                  >
                    <Select
                      value={platformSource}
                      triggerProps={{ "aria-label": t("appPlatformCredentialSource") }}
                      options={[
                        { value: "manual", label: t("appManualPlatformToken") },
                        { value: "opencsg_login", label: t("appUseOpenCSGLogin") },
                      ]}
                      onValueChange={(value) =>
                        updateConfig({ platform_credential_source: value as AppConfig["platform_credential_source"] })
                      }
                    />
                  </Field>
                ) : null}
              </div>
              {!gitlab && oauthUnsupported ? (
                <p className="form-warning" role="status">
                  {t("appOAuthUnsupported")}
                </p>
              ) : null}
              {appCredentials && !stdio && platformSource !== "opencsg_login" ? (
                <Field label={t("appPlatformToken")} hint={t("appFeishuTokenHint")}>
                  <TextInput
                    aria-label={t("appPlatformToken")}
                    type="password"
                    autoComplete="new-password"
                    value={form.credentials.token || ""}
                    placeholder={credentialPlaceholder("token")}
                    onChange={(event) =>
                      setForm({
                        ...form,
                        config: {
                          ...form.config,
                          platform_credential_source:
                            config.auth_mode === "header" ? form.config.platform_credential_source : "manual",
                        },
                        credentials: { ...form.credentials, token: event.target.value },
                      })
                    }
                  />
                </Field>
              ) : null}
              {gitlab ? (
                <Field required label={t("connectorGitLabToken")} hint={t("connectorGitLabTokenKeep")}>
                  <TextInput
                    aria-label={t("connectorGitLabToken")}
                    required={!hasSavedCredential("token")}
                    placeholder={credentialPlaceholder("token")}
                    type="password"
                    autoComplete="new-password"
                    value={form.credentials.token || ""}
                    onChange={(event) =>
                      setForm({ ...form, credentials: { ...form.credentials, token: event.target.value } })
                    }
                  />
                </Field>
              ) : !appCredentials &&
                config.auth_mode !== "none" &&
                config.auth_mode !== "oauth2" &&
                config.auth_mode !== "connector" &&
                (config.auth_mode === "header" || platformSource !== "opencsg_login") ? (
                <Field required label={t("appToken")} hint={t("appSecretHint")}>
                  <TextInput
                    aria-label={t("appToken")}
                    required={!hasSavedCredential("token")}
                    type="password"
                    autoComplete="new-password"
                    value={form.credentials.token || ""}
                    placeholder={credentialPlaceholder("token")}
                    onChange={(event) =>
                      setForm({
                        ...form,
                        config: {
                          ...form.config,
                          platform_credential_source:
                            config.auth_mode === "header" ? form.config.platform_credential_source : "manual",
                        },
                        credentials: { ...form.credentials, token: event.target.value },
                      })
                    }
                  />
                </Field>
              ) : null}
              {config.auth_mode === "header" ? (
                <div className={styles.columns}>
                  <Field label={t("appTokenHeader")}>
                    <TextInput
                      value={config.token_header || ""}
                      onChange={(event) => updateConfig({ token_header: event.target.value })}
                      placeholder="X-API-Key"
                    />
                  </Field>
                  <Field label={t("appTokenPrefix")}>
                    <TextInput
                      value={config.token_prefix || ""}
                      onChange={(event) => updateConfig({ token_prefix: event.target.value })}
                      placeholder="Bearer "
                    />
                  </Field>
                </div>
              ) : null}
              {config.auth_mode === "env" ? (
                <Field label={t("appTokenEnvironment")}>
                  <TextInput
                    value={config.token_env || ""}
                    onChange={(event) => updateConfig({ token_env: event.target.value })}
                  />
                </Field>
              ) : null}
            </section>
            {appCredentials ? (
              <section className={styles.formSection} aria-label={t("appFeishuIdentitySection")}>
                <header className={styles.sectionHeading}>
                  <h3>{t("appFeishuIdentitySection")}</h3>
                  <p>{t("appGlobalCredentialHint")}</p>
                </header>
                <Field required label="App ID">
                  <TextInput
                    aria-label="App ID"
                    required
                    autoComplete="off"
                    value={form.credentials.app_id || ""}
                    onChange={(event) =>
                      setForm({ ...form, credentials: { ...form.credentials, app_id: event.target.value } })
                    }
                  />
                </Field>
                <Field required label="App Secret" hint={t("appSecretHint")}>
                  <TextInput
                    aria-label="App Secret"
                    required={!hasSavedCredential("app_secret")}
                    type="password"
                    autoComplete="new-password"
                    value={form.credentials.app_secret || ""}
                    placeholder={credentialPlaceholder("app_secret")}
                    onChange={(event) =>
                      setForm({ ...form, credentials: { ...form.credentials, app_secret: event.target.value } })
                    }
                  />
                </Field>
                {stdio ? (
                  <div className={styles.columns}>
                    <Field label={t("appIDEnvironment")}>
                      <TextInput
                        value={config.app_id_env || ""}
                        onChange={(event) => updateConfig({ app_id_env: event.target.value })}
                      />
                    </Field>
                    <Field label={t("appSecretEnvironment")}>
                      <TextInput
                        value={config.app_secret_env || ""}
                        onChange={(event) => updateConfig({ app_secret_env: event.target.value })}
                      />
                    </Field>
                  </div>
                ) : null}
              </section>
            ) : null}
            <details className={styles.advanced}>
              <summary>{t("appAdvancedSettings")}</summary>
              <div className={styles.fields}>
                <div className={styles.columns}>
                  <Field label={t("appStartupTimeout")}>
                    <TextInput
                      type="number"
                      min={1}
                      value={config.startup_timeout_sec || ""}
                      onChange={(event) =>
                        updateConfig({ startup_timeout_sec: Number(event.target.value) || undefined })
                      }
                    />
                  </Field>
                  <Field label={t("appToolTimeout")}>
                    <TextInput
                      type="number"
                      min={1}
                      value={config.tool_timeout_sec || ""}
                      onChange={(event) => updateConfig({ tool_timeout_sec: Number(event.target.value) || undefined })}
                    />
                  </Field>
                </div>
                <AppValueFields
                  label={stdio ? t("appExtraEnvironment") : t("appExtraHeaders")}
                  rows={stdio ? form.env : form.headers}
                  t={t}
                  onChange={(rows) => setForm({ ...form, [stdio ? "env" : "headers"]: rows })}
                />
                {gitlab ? <small className={styles.hint}>{t("appGitLabHeadersHint")}</small> : null}
                {existing &&
                Object.keys(existing.credentials_set).some(
                  (key) => key.startsWith("env") || key.startsWith("header"),
                ) ? (
                  <small className={styles.hint}>{t("appCustomSecretsSaved")}</small>
                ) : null}
              </div>
            </details>
          </fieldset>
          {error ? (
            <div className="form-error" role="alert">
              {error}
            </div>
          ) : null}
          {tested ? (
            <div className={styles.probeResult} role="status">
              <strong>{t("appProbeSucceeded", { count: probe.result.tools.length })}</strong>
              <p className={styles.hint}>{t("appProbeNotSaved")}</p>
            </div>
          ) : null}
          {probe && !tested ? <p className={styles.hint}>{t("appProbeStale")}</p> : null}
          <div ref={probeResultRef}>
            <AppToolList
              key={tested ? "tested" : "saved"}
              tools={
                tested ? probe.result.tools : signature === initialSignature && !error ? (existing?.tools ?? []) : []
              }
              t={t}
            />
          </div>
        </DialogBody>
        <DialogFooter className={styles.footer}>
          <Button variant="secondaryGray" disabled={Boolean(busy)} onClick={onClose}>
            {t("cancel")}
          </Button>
          {!globalResource ? (
            <Button
              disabled={Boolean(busy) || !requiredFieldsFilled || oauthUnsupported}
              loading={busy === "save"}
              onClick={() => void execute("save")}
            >
              {t("appSaveConfiguration")}
            </Button>
          ) : null}
          <Button
            variant="secondaryGray"
            disabled={Boolean(busy) || !requiredFieldsFilled || oauthUnsupported}
            loading={busy === "probe"}
            onClick={() => void execute("probe")}
          >
            {t("appTestConnection")}
          </Button>
          <Button
            variant="primary"
            disabled={Boolean(busy) || !requiredFieldsFilled || !tested || oauthUnsupported}
            loading={busy === (globalResource ? "save" : "connect")}
            onClick={() => void execute(globalResource ? "save" : "connect")}
          >
            {globalResource ? t("appSaveConfiguration") : existing ? t("appSaveAndConnect") : t("appAddAndConnect")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

function AppValueFields({
  label,
  rows,
  t,
  onChange,
}: {
  label: string;
  rows: AppValueRow[];
  t: TranslateFn;
  onChange: (rows: AppValueRow[]) => void;
}) {
  return (
    <div className={styles.fields}>
      <strong className={styles.smallTitle}>{label}</strong>
      {rows.map((row, index) => (
        <div className={styles.valueRow} key={index}>
          <TextInput
            aria-label={t("appVariableName", { index: index + 1 })}
            value={row.key}
            onChange={(event) =>
              onChange(rows.map((item, i) => (i === index ? { ...item, key: event.target.value } : item)))
            }
            placeholder={t("appVariableKey")}
          />
          <TextInput
            type="password"
            autoComplete="new-password"
            aria-label={t("appVariableValue", { index: index + 1 })}
            value={row.value}
            onChange={(event) =>
              onChange(rows.map((item, i) => (i === index ? { ...item, value: event.target.value } : item)))
            }
            placeholder={t("appVariableValueLabel")}
          />
          <Button
            variant="tertiaryDanger"
            aria-label={t("appRemoveVariable", { index: index + 1 })}
            onClick={() => onChange(rows.filter((_, i) => i !== index))}
          >
            ×
          </Button>
        </div>
      ))}
      <Button className={styles.addValue} size="sm" onClick={() => onChange([...rows, { key: "", value: "" }])}>
        {t("appAddVariable")}
      </Button>
    </div>
  );
}

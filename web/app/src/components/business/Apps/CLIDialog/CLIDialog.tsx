import { useEffect, useRef, useState } from "react";
import { CircleAlert } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import {
  createAppResource,
  type AppDefinition,
  fetchAppResource,
  startCLIOperation,
  cancelCLIOperation,
  type AppInstallation,
  type CLIAction,
} from "@/api/apps";
import {
  Button,
  DialogRoot,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
  DialogCloseButton,
  DismissibleAlert,
} from "@/components/ui";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIError } from "@/shared/i18n";
import { errorMessage } from "@/api/client";
import { appStatus } from "../appForm";
import styles from "../AgentAppsPanel.module.css";
import stepPending from "./assets/step-pending.svg";
import stepCurrent from "./assets/step-current.svg";
import stepCompleted from "./assets/step-completed.svg";

type Props = {
  definition: AppDefinition;
  existing: AppInstallation | null;
  t: TranslateFn;
  onClose: () => void;
  onChanged: () => Promise<unknown>;
};

export function CLIDialog({ definition, existing, t, onClose, onChanged }: Props) {
  const feishu = definition.app_id === "feishu";
  const [resource, setResource] = useState(existing);
  const [busy, setBusy] = useState(false);
  const [cancellingSession, setCancellingSession] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [showQRCode, setShowQRCode] = useState(false);
  const retry = useRef<{ action: CLIAction; id: string; revision: number | undefined } | null>(null);
  const resourceID = resource?.installation_id;
  const migrationWorkers = resource?.migration?.workers ?? [];
  useEffect(() => {
    if (!resourceID) return;
    const abort = new AbortController();
    let pending = false;
    const refresh = async () => {
      if (pending) return;
      pending = true;
      try {
        const value = await fetchAppResource(resourceID, abort.signal);
        if (!abort.signal.aborted) {
          setResource(value);
          if (
            cancellingSession &&
            (value.operation?.session_id !== cancellingSession ||
              (value.operation.status !== "running" && value.operation.status !== "pending"))
          ) {
            setCancellingSession(null);
            await onChanged();
          }
        }
      } catch (failure) {
        if (!abort.signal.aborted) {
          setError(failure);
          setCancellingSession(null);
        }
      } finally {
        pending = false;
      }
    };
    void refresh();
    const timer = window.setInterval(
      () => {
        if (!document.hidden) void refresh();
      },
      cancellingSession ? 100 : 1500,
    );
    const refreshOnReturn = () => {
      if (!document.hidden) void refresh();
    };
    window.addEventListener("focus", refreshOnReturn);
    document.addEventListener("visibilitychange", refreshOnReturn);
    return () => {
      abort.abort();
      window.clearInterval(timer);
      window.removeEventListener("focus", refreshOnReturn);
      document.removeEventListener("visibilitychange", refreshOnReturn);
    };
  }, [resourceID, cancellingSession, onChanged]);
  async function run(action: CLIAction) {
    setBusy(true);
    setError(null);
    try {
      const current =
        resource ??
        (await createAppResource({
          app_id: definition.app_id,
          kind: "cli",
          name: definition.name,
          config: {},
          credentials: {},
          connect: false,
        }));
      setResource(current);
      if (retry.current?.action !== action || retry.current.revision !== current.revision) {
        retry.current = {
          action,
          id:
            typeof crypto.randomUUID === "function"
              ? crypto.randomUUID()
              : `cli-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`,
          revision: current.revision,
        };
      }
      const operation = await startCLIOperation(current, action, retry.current.id);
      retry.current = null;
      setResource({ ...current, operation });
      await onChanged();
    } catch (failure) {
      setError(failure);
    } finally {
      setBusy(false);
    }
  }
  async function cancel() {
    if (!resource?.operation) return;
    setBusy(true);
    try {
      const operation = await cancelCLIOperation(resource.installation_id, resource.operation.session_id);
      setResource({ ...resource, operation });
      setCancellingSession(resource.operation.session_id);
    } catch (failure) {
      setError(failure);
    } finally {
      setBusy(false);
    }
  }
  const operation = resource?.operation;
  const actionPending = busy || cancellingSession !== null;
  const active = operation?.status === "running" || operation?.status === "pending";
  const ready = Boolean(resource?.authorized && !resource.disconnected);
  const identityAuthorized = (identity: "bot" | "user") => {
    const value = resource?.auth_status?.identities[identity];
    return Boolean(value?.available && value.verified === true);
  };
  const activeIdentity =
    operation?.stage === "app_authorization" ? "bot" : operation?.stage === "user_authorization" ? "user" : undefined;
  const stages: Record<string, string> = {
    authorization: "cliAuthorization",
    checking_authorization: "cliCheckingAuthorization",
    pending: "feishuCLIPreparing",
    checking_environment: "feishuCLIPreparing",
    downloading_cli: "feishuCLIInstalling",
    downloading_skills: "feishuCLIInstalling",
    installing_skills: "feishuCLIInstalling",
    verifying: "feishuCLIInstalling",
    verifying_installation: "feishuCLIInstalling",
    app_authorization: "feishuCLIAppAuthorization",
    user_authorization: "feishuCLIUserAuthorization",
    applying: "feishuCLIApplying",
    completed: "feishuCLICompleted",
  };
  const errorLabels: Record<string, string> = {
    authorization_expired: "feishuCLIExpired",
    authorization_denied: "feishuCLIDenied",
    permission_denied: "feishuCLIPermissionDenied",
    cleanup_pending: "feishuCLICleanupPending",
    migration_failed: "feishuCLIMigrationFailed",
  };
  const requestError = error ? localizeAPIError(error, t) || errorMessage(error) : "";
  const storedError =
    resource?.last_error_code && !active ? t(errorLabels[resource.last_error_code] ?? "feishuCLIRepairHint") : "";
  return (
    <DialogRoot
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className={styles.settingsDialog}>
        <DialogHeader>
          <div>
            <DialogTitle>{definition.name}</DialogTitle>
            <DialogDescription>{feishu ? t("feishuCLIIntro") : definition.description}</DialogDescription>
          </div>
          <DialogCloseButton label={t("close")} variant="tertiaryGray" size="sm" iconOnly />
        </DialogHeader>
        <DialogBody className={styles.settingsBody}>
          <div className={styles.cliStatus}>
            {requestError ? (
              <DismissibleAlert
                className={`${styles.cliError} ${styles.resourceError}`}
                messageKey={requestError}
                closeLabel={t("close")}
              >
                <CircleAlert size={16} aria-hidden="true" />
                <span role="alert">{requestError}</span>
              </DismissibleAlert>
            ) : null}
            {storedError ? (
              <DismissibleAlert
                className={`${styles.cliError} ${styles.resourceError}`}
                messageKey={`${resource?.last_error_code}:${storedError}`}
                closeLabel={t("close")}
              >
                <CircleAlert size={16} aria-hidden="true" />
                <span role="alert">{storedError}</span>
              </DismissibleAlert>
            ) : null}
            <p role="status" aria-live="polite">
              {active
                ? t(stages[operation?.stage ?? ""] ?? "feishuCLIPreparing")
                : resource
                  ? appStatus(resource, t)
                  : t("appStatusNeedsConfiguration")}
            </p>
          </div>
          {feishu ? (
            <section className={styles.authorizationSteps} aria-label={t("feishuCLIAuthorizationSteps")}>
              <div className={styles.authorizationStepsHeader}>
                <strong>{t("feishuCLIAuthorizationSteps")}</strong>
                <p>{t("feishuCLIAuthorizationStepsHint")}</p>
              </div>
              {(["bot", "user"] as const).map((identity) => {
                const info = resource?.auth_status?.identities[identity];
                const completed = identityAuthorized(identity);
                const current = active && !completed && activeIdentity === identity;
                const state = completed ? "completed" : current ? "current" : "pending";
                const label = t(identity === "bot" ? "feishuCLIBot" : "feishuCLIUser");
                return (
                  <div key={identity} className={styles.authorizationStep} data-state={state}>
                    <span className={styles.authorizationStepNumber} aria-hidden="true">
                      <img src={completed ? stepCompleted : current ? stepCurrent : stepPending} alt="" />
                    </span>
                    <span className={styles.authorizationStepCopy}>
                      <strong>
                        {label}
                        <span className={styles.authorizationStepState}>
                          {t(
                            completed
                              ? "feishuCLIStepCompleted"
                              : current
                                ? "feishuCLIStepCurrent"
                                : "feishuCLIStepPending",
                          )}
                        </span>
                      </strong>
                      <span>
                        {info?.appName ||
                          info?.userName ||
                          t(identity === "bot" ? "feishuCLIBotHint" : "feishuCLIUserHint")}
                      </span>
                    </span>
                    {current && operation?.authorization_url ? (
                      <span className={styles.authorizationStepActions}>
                        {feishu ? (
                          <button
                            type="button"
                            className="btn btn-tertiary-gray btn-sm"
                            aria-expanded={showQRCode}
                            onClick={() => setShowQRCode((visible) => !visible)}
                          >
                            {t(showQRCode ? "feishuCLIHideQRCode" : "feishuCLIShowQRCode")}
                          </button>
                        ) : null}
                        <a
                          href={operation.authorization_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="btn btn-primary btn-sm"
                        >
                          {t(identity === "bot" ? "feishuCLIAuthorizeBot" : "feishuCLIAuthorizeUser")}
                        </a>
                      </span>
                    ) : null}
                  </div>
                );
              })}
            </section>
          ) : null}
          {operation?.authorization_url && active ? (
            <div className="flex flex-col items-start gap-3">
              {(!feishu || showQRCode) && (
                <QRCodeSVG
                  value={operation.authorization_url}
                  size={168}
                  marginSize={2}
                  title={t(feishu ? "feishuCLIAuthorize" : "cliAuthorize")}
                />
              )}
              <p className={styles.hint}>{t(feishu ? "feishuCLIWaitHint" : "cliWaitHint")}</p>
            </div>
          ) : null}
          {resource?.migration && migrationWorkers.length > 0 ? (
            <div>
              <p>
                {t(
                  resource.migration.status === "completed"
                    ? "feishuCLIMigrationCompleted"
                    : "feishuCLIMigrationPending",
                )}
              </p>
              <ul>
                {migrationWorkers.map((worker) => (
                  <li key={worker.agent_id}>
                    {worker.agent_id} ·{" "}
                    {t(
                      worker.error_code
                        ? "feishuCLIMigrationFailed"
                        : worker.verification_pending
                          ? "feishuCLIVerificationPending"
                          : worker.phase === "completed"
                            ? "feishuCLICompleted"
                            : "feishuCLIMigrationPending",
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {resource?.checked_at && !resource.checked_at.startsWith("0001") ? (
            <p className={styles.hint}>
              {t("feishuCLICheckedAt", { time: new Date(resource.checked_at).toLocaleString() })}
            </p>
          ) : null}
          {resource?.package_version ? (
            <p className={styles.hint}>{t("feishuCLIVersion", { version: resource.package_version })}</p>
          ) : null}
          <p className={styles.hint}>{t("appResourceUsers", { count: resource?.bindings?.length ?? 0 })}</p>
          <ul className={styles.bindingList}>
            {resource?.bindings?.map((binding) => (
              <li key={binding.installation_id}>
                <a
                  href={`#/agents/${encodeURIComponent(binding.agent_id)}?tab=connectors&connector=${encodeURIComponent(binding.installation_id)}`}
                >
                  {binding.agent_name || binding.agent_id}
                </a>
                <span>
                  {appStatus({ ...resource, enabled: binding.enabled, status: binding.status, disconnected: false }, t)}
                  {binding.runtime_status === "configured"
                    ? ` · ${t("feishuCLIConfiguredRuntime")}`
                    : binding.runtime_status === "loaded"
                      ? ` · ${t("feishuCLILoadedRuntime")}`
                      : ""}
                </span>
              </li>
            ))}
          </ul>
        </DialogBody>
        <DialogFooter className={styles.cliFooter}>
          {active ? (
            <>
              <Button disabled={actionPending} onClick={() => void cancel()}>
                {t("cancel")}
              </Button>
              {!feishu && operation?.authorization_url ? (
                <div className={styles.cliFooterActions}>
                  <a
                    href={operation.authorization_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="btn btn-primary btn-sm"
                  >
                    {t("cliAuthorize")}
                  </a>
                </div>
              ) : null}
            </>
          ) : (
            <>
              {ready ? (
                <div className={styles.cliFooterActions}>
                  {feishu && resource?.auth_status?.identities.user.verified !== true ? (
                    <Button disabled={actionPending} onClick={() => void run("login")}>
                      {t("feishuCLILogin")}
                    </Button>
                  ) : null}
                  <Button variant="danger" disabled={actionPending} onClick={() => void run("disconnect")}>
                    {t("appDisconnect")}
                  </Button>
                </div>
              ) : null}
              {!ready ? (
                <Button
                  className={styles.cliConnectAction}
                  variant="primary"
                  disabled={actionPending}
                  onClick={() => void run("connect")}
                >
                  {t(feishu ? "feishuCLIConnect" : "appConnect")}
                </Button>
              ) : null}
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </DialogRoot>
  );
}

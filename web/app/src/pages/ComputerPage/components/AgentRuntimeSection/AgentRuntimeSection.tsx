import type { ReactNode } from "react";
import { AlertCircle, CheckCircle2, CircleDashed, Clock3, LoaderCircle, RefreshCw, SquareTerminal } from "lucide-react";
import { Button, Tooltip } from "@/components/ui";
import { RuntimeInstallProgress } from "@/components/business";
import type { AgentRuntimeInstallation } from "@/api/agentRuntimes";
import { AgentRuntimeStatuses } from "@/models/agentRuntimes";
import type { AgentRuntime, AgentRuntimeStatus } from "@/models/agentRuntimes";
import type { TranslateFn } from "@/models/conversations";
import { classNames } from "@/shared/lib/classNames";
import styles from "./AgentRuntimeSection.module.css";

type VoidOrPromise = void | Promise<void>;

export type AgentRuntimeSectionProps = {
  error?: string;
  installError?: string;
  installProgress?: AgentRuntimeInstallation | null;
  installingRuntime?: string;
  loading?: boolean;
  onInstallRuntime?: (name: string) => VoidOrPromise;
  onRetryLoad?: () => VoidOrPromise;
  refreshing?: boolean;
  runtimes?: AgentRuntime[];
  t?: TranslateFn;
};

const runtimeLogos: Record<string, string> = {
  codex: "model-providers/codex.svg",
  claude_code: "model-providers/claude-code.svg",
};

export function AgentRuntimeSection({
  error = "",
  installError = "",
  installProgress = null,
  installingRuntime = "",
  loading = false,
  onInstallRuntime = () => {},
  onRetryLoad = () => {},
  refreshing = false,
  runtimes = [],
  t = (key) => key,
}: AgentRuntimeSectionProps) {
  return (
    <section className={styles.panel} aria-labelledby="computer-agent-runtimes-title">
      <header className={styles.header}>
        <div>
          <h2 id="computer-agent-runtimes-title">{t("computerRuntimesTitle")}</h2>
          <p>{t("computerRuntimesSubtitle")}</p>
        </div>
        {refreshing && runtimes.length ? (
          <span className={styles.refreshing} role="status" aria-live="polite">
            <LoaderCircle className={styles.spinner} size={15} aria-hidden="true" />
            {t("computerRuntimesRefreshing")}
          </span>
        ) : null}
      </header>

      {loading && !runtimes.length ? (
        <div className={styles.loadingState} role="status" aria-live="polite">
          <LoaderCircle className={styles.spinner} size={20} aria-hidden="true" />
          <span>{t("computerRuntimesLoading")}</span>
        </div>
      ) : (
        <>
          {error ? (
            <div className={styles.loadError} role="alert">
              <span className={styles.loadErrorMessage}>
                <AlertCircle size={17} aria-hidden="true" />
                {error}
              </span>
              <Button
                variant="secondaryGray"
                size="sm"
                loading={refreshing}
                loadingLabel={t("computerRuntimeRetry")}
                onClick={() => void onRetryLoad()}
              >
                <RefreshCw size={15} aria-hidden="true" />
                {t("computerRuntimeRetry")}
              </Button>
            </div>
          ) : null}

          {runtimes.length ? (
            <ul className={styles.grid}>
              {runtimes.map((runtime) => (
                <RuntimeCard
                  key={runtime.name}
                  runtime={runtime}
                  installError={runtime.name === "dsh" ? installError : ""}
                  installation={runtime.name === installingRuntime ? installProgress : null}
                  installing={installingRuntime === runtime.name}
                  onInstallRuntime={onInstallRuntime}
                  t={t}
                />
              ))}
            </ul>
          ) : error ? null : (
            <div className={styles.emptyState}>{t("computerRuntimesEmpty")}</div>
          )}
        </>
      )}
    </section>
  );
}

function RuntimeCard({
  runtime,
  installError,
  installation,
  installing,
  onInstallRuntime,
  t,
}: {
  runtime: AgentRuntime;
  installError: string;
  installation: AgentRuntimeInstallation | null;
  installing: boolean;
  onInstallRuntime: (name: string) => VoidOrPromise;
  t: TranslateFn;
}) {
  const status = installing ? AgentRuntimeStatuses.installing : runtimeStatus(runtime);
  const statusMeta = runtimeStatusMeta(status, t);
  const visibleError = runtime.installed
    ? ""
    : installError || (!runtime.installable ? localizedRuntimeMessage(runtime, t) : "");
  const logo = runtimeLogos[runtime.name];

  return (
    <li
      className={classNames(
        styles.card,
        runtime.name === "codex" && styles.codexCard,
        runtime.name === "claude_code" && styles.claudeCard,
      )}
    >
      <div className={styles.cardHeader}>
        <span className={styles.logo} aria-hidden="true">
          {logo ? <img src={logo} alt="" /> : <SquareTerminal size={24} />}
        </span>
        <div className={styles.identity}>
          <h3>{runtime.label}</h3>
          <p>{runtimeDescription(runtime.name, t)}</p>
        </div>
        <span className={classNames(styles.status, styles[statusMeta.tone])}>
          {statusMeta.icon}
          {statusMeta.label}
        </span>
      </div>

      <div className={styles.cardBody}>
        {runtime.installed && runtime.path ? (
          <div className={styles.pathRow}>
            <SquareTerminal size={15} aria-hidden="true" />
            <span>{t("computerRuntimeExecutable")}</span>
            <Tooltip content={runtime.path}>
              <code>{runtime.path}</code>
            </Tooltip>
          </div>
        ) : null}
        {runtime.installed && runtime.version ? (
          <div className={styles.pathRow}>
            <span>{t("computerRuntimeVersion")}</span>
            <code>{runtime.version}</code>
          </div>
        ) : null}
        {installing ? <RuntimeInstallProgress installation={installation} t={t} /> : null}
        {!installing && !runtime.installed && runtime.installable && !visibleError ? (
          <div className={styles.installNotice} role="status">
            <SquareTerminal size={16} aria-hidden="true" />
            <span>{t("computerRuntimeManagedInstallDescription")}</span>
          </div>
        ) : null}
        {visibleError ? (
          <div className={styles.runtimeError} role="alert">
            <AlertCircle size={16} aria-hidden="true" />
            <span>{visibleError}</span>
          </div>
        ) : null}
      </div>

      <footer className={styles.cardFooter}>
        <span>{runtimeHint(runtime, status, t)}</span>
        <span className={styles.footerActions}>
          {!runtime.installed && runtime.docsURL ? (
            <a href={runtime.docsURL} target="_blank" rel="noreferrer">
              {t("computerRuntimeDocs")}
            </a>
          ) : null}
          {!runtime.installed && runtime.installable ? (
            <Button
              variant="primary"
              size="sm"
              loading={installing}
              loadingLabel={t("computerRuntimeInstalling")}
              onClick={() => void onInstallRuntime(runtime.name)}
            >
              {t("computerRuntimeInstall")}
            </Button>
          ) : null}
        </span>
      </footer>
    </li>
  );
}

function localizedRuntimeMessage(runtime: AgentRuntime, t: TranslateFn): string {
  if (!runtime.messageCode) {
    return runtime.message || "";
  }
  const key = `errors.${runtime.messageCode}`;
  const localized = t(key);
  return localized === key ? runtime.message || "" : localized;
}

function runtimeStatus(runtime: AgentRuntime): AgentRuntimeStatus {
  if (runtime.installed) {
    return AgentRuntimeStatuses.installed;
  }
  return runtime.status;
}

function runtimeStatusMeta(
  status: AgentRuntimeStatus,
  t: TranslateFn,
): { icon: ReactNode; label: string; tone: "success" | "neutral" | "progress" | "danger" } {
  switch (status) {
    case AgentRuntimeStatuses.installed:
      return {
        icon: <CheckCircle2 size={14} aria-hidden="true" />,
        label: t("computerRuntimeInstalled"),
        tone: "success",
      };
    case AgentRuntimeStatuses.installing:
      return {
        icon: <LoaderCircle className={styles.spinner} size={14} aria-hidden="true" />,
        label: t("computerRuntimeInstalling"),
        tone: "progress",
      };
    case AgentRuntimeStatuses.failed:
      return {
        icon: <AlertCircle size={14} aria-hidden="true" />,
        label: t("computerRuntimeFailed"),
        tone: "danger",
      };
    case AgentRuntimeStatuses.comingSoon:
      return {
        icon: <Clock3 size={14} aria-hidden="true" />,
        label: t("computerRuntimeComingSoon"),
        tone: "neutral",
      };
    case AgentRuntimeStatuses.unsupported:
      return {
        icon: <CircleDashed size={14} aria-hidden="true" />,
        label: t("computerRuntimeUnsupported"),
        tone: "neutral",
      };
    default:
      return {
        icon: <CircleDashed size={14} aria-hidden="true" />,
        label: t("computerRuntimeNotInstalled"),
        tone: "neutral",
      };
  }
}

function runtimeDescription(runtimeName: string, t: TranslateFn): string {
  switch (runtimeName) {
    case "codex":
      return t("computerRuntimeCodexDescription");
    case "dsh":
      return t("computerRuntimeDSHDescription");
    case "claude_code":
      return t("computerRuntimeClaudeDescription");
    default:
      return t("computerRuntimeDescription");
  }
}

function runtimeHint(runtime: AgentRuntime, status: AgentRuntimeStatus, t: TranslateFn): string {
  switch (status) {
    case AgentRuntimeStatuses.installed:
      return t("computerRuntimeReadyHint");
    case AgentRuntimeStatuses.installing:
      return t("computerRuntimeInstallingHint");
    case AgentRuntimeStatuses.failed:
      return t("computerRuntimeBundleMissingHint");
    case AgentRuntimeStatuses.comingSoon:
      return t("computerRuntimeComingSoonHint");
    case AgentRuntimeStatuses.unsupported:
      return t("computerRuntimeUnsupportedHint");
    default:
      return runtime.installable ? t("computerRuntimeInstallHint") : t("computerRuntimeExternalInstallHint");
  }
}

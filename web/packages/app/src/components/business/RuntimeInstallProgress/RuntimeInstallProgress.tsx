import { useEffect, useMemo, useState } from "react";
import type { AgentRuntimeInstallation } from "@/api/agentRuntimes";
import type { TranslateFn } from "@/models/conversations";
import { classNames } from "@/shared/lib/classNames";
import styles from "./RuntimeInstallProgress.module.css";

const installStages = [
  "checking_environment",
  "installing_node",
  "installing_packages",
  "creating_launcher",
  "verifying_installation",
  "configuring_path",
] as const;

export function RuntimeInstallProgress({
  installation,
  t,
}: {
  installation: AgentRuntimeInstallation | null | undefined;
  t: TranslateFn;
}) {
  const [now, setNow] = useState(() => Date.now());
  const status = installation?.status || "running";
  const stage = installation?.stage || installStages[0];
  const stageIndex = installStages.findIndex((item) => item === stage);
  const currentIndex = status === "succeeded" ? installStages.length : Math.max(0, stageIndex);
  const stageLabel = runtimeInstallStageLabel(stage, t);
  const elapsedSeconds = useMemo(() => {
    const startedAt = Date.parse(installation?.startedAt || "");
    return Number.isFinite(startedAt) ? Math.max(0, Math.floor((now - startedAt) / 1000)) : 0;
  }, [installation?.startedAt, now]);

  useEffect(() => {
    if (status !== "running") {
      return undefined;
    }
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [status]);

  return (
    <div className={styles.root} aria-live="polite">
      <div
        className={styles.track}
        role="progressbar"
        aria-label={t("computerRuntimeInstallProgressLabel")}
        aria-valuemin={0}
        aria-valuemax={installStages.length}
        aria-valuenow={currentIndex}
        aria-valuetext={stageLabel}
      >
        {installStages.map((item, index) => (
          <span
            key={item}
            className={classNames(
              styles.segment,
              index < currentIndex && styles.complete,
              status === "running" && index === currentIndex && styles.active,
            )}
          />
        ))}
      </div>
      <div className={styles.meta}>
        <strong>{stageLabel}</strong>
        <span>
          {installation?.activityCount && stage === "installing_packages"
            ? `${t("computerRuntimeInstallActivity", { count: installation.activityCount })} · ${t(
                "computerRuntimeInstallElapsed",
                { seconds: elapsedSeconds },
              )}`
            : t("computerRuntimeInstallElapsed", { seconds: elapsedSeconds })}
        </span>
      </div>
      {stage === "installing_packages" ? <small>{t("computerRuntimeInstallFirstRunHint")}</small> : null}
    </div>
  );
}

function runtimeInstallStageLabel(stage: string, t: TranslateFn): string {
  switch (stage) {
    case "installing_node":
      return t("computerRuntimeInstallStageInstallingNode");
    case "installing_packages":
      return t("computerRuntimeInstallStageInstalling");
    case "creating_launcher":
      return t("computerRuntimeInstallStageLinking");
    case "verifying_installation":
      return t("computerRuntimeInstallStageVerifying");
    case "configuring_path":
      return t("computerRuntimeInstallStageConfiguring");
    case "completed":
      return t("computerRuntimeInstallStageCompleted");
    default:
      return t("computerRuntimeInstallStageChecking");
  }
}

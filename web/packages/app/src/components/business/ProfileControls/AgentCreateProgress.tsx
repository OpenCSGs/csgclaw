import type { AgentCreateProgressState, Translator } from "./types";

export type AgentCreateProgressProps = {
  progress?: AgentCreateProgressState | null;
  t: Translator;
};

export function AgentCreateProgress({ progress, t }: AgentCreateProgressProps) {
  if (!progress) {
    return null;
  }
  const steps = progress.steps || [];
  const progressIndex = progress.index ?? 0;
  const currentStep = steps[Math.min(progressIndex, Math.max(steps.length - 1, 0))];
  const failed = progress.status === "failed";
  const done = progress.status === "done";
  const label = failed
    ? t("agentCreateProgressFailed")
    : done
      ? t("agentCreateProgressDone")
      : t(currentStep?.label || "agentCreateProgressPreparing");
  const percent = Math.max(0, Math.min(100, Math.round(progress.percent || 0)));
  return (
    <div
      className={`agent-create-progress ${failed ? "failed" : ""} ${done ? "done" : ""}`.trim()}
      role="status"
      aria-live="polite"
    >
      <div className="agent-create-progress-header">
        <span>{label}</span>
        <strong>{percent}%</strong>
      </div>
      <div className="agent-create-progress-track" aria-hidden="true">
        <div className="agent-create-progress-fill" style={{ width: `${percent}%` }} />
      </div>
      <div className="agent-create-progress-steps">
        {steps.map((step, index) => (
          <span
            key={`${step.label}-${index}`}
            className={index < progressIndex || done ? "complete" : index === progressIndex && !failed ? "active" : ""}
          >
            {t(step.label)}
          </span>
        ))}
      </div>
    </div>
  );
}

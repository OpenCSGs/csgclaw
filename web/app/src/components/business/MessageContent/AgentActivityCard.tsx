import { useEffect, useMemo, useState } from "react";
import { Check, CircleHelp, Clock3, LockKeyhole, ShieldAlert, ShieldCheck, ShieldX, X } from "lucide-react";
import { decideChannelActivity } from "@/api/agentActivities";
import { errorMessage } from "@/api/client";
import { AgentActivityMsgTypes } from "@/shared/constants/messages";
import { actionOptionLabel, questionOptions, statusLabel } from "@/models/agentActivity";
import { renderMarkdown } from "./markdown";
import type {
  AgentActivityAction,
  AgentActivityActionOption,
  AgentActivityPayload,
  AgentActivityQuestion,
} from "@/models/agentActivity";
import type { TranslateFn } from "@/models/conversations";
import { Button } from "@/components/ui";

type AgentActivityCardProps = {
  activity: AgentActivityPayload;
  onQuestionSelect?: (activityID: string, questionID?: string, optionIndex?: number) => void;
  t?: TranslateFn;
};

export function AgentActivityCard({ activity, onQuestionSelect, t }: AgentActivityCardProps) {
  if (activity.content.msgtype === AgentActivityMsgTypes.tool && activity.content.tool) {
    return <ToolActivityCard activity={activity} />;
  }
  if (activity.content.msgtype === AgentActivityMsgTypes.action && activity.content.action?.kind === "permission") {
    return <PermissionActivityCard activity={activity} action={activity.content.action} />;
  }
  if (activity.content.msgtype === AgentActivityMsgTypes.question && activity.content.question) {
    return (
      <QuestionActivityCard question={activity.content.question} onSelect={onQuestionSelect} t={t ?? ((key) => key)} />
    );
  }
  return <NoticeActivityCard body={activity.content.body} />;
}

function QuestionActivityCard({
  question,
  onSelect,
  t,
}: {
  question: AgentActivityQuestion;
  onSelect?: (activityID: string, questionID?: string, optionIndex?: number) => void;
  t: TranslateFn;
}) {
  const pending = question.status === "pending";
  const deadline = question.auto_resolve_at ? new Date(question.auto_resolve_at) : null;
  const deadlineLabel =
    deadline && !Number.isNaN(deadline.valueOf())
      ? deadline.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
      : "";
  return (
    <section className={`agent-activity-card agent-activity-question status-${question.status}`}>
      <div className="agent-activity-header">
        <span className="agent-activity-icon" aria-hidden="true">
          <CircleHelp size={17} />
        </span>
        <div className="agent-activity-title-group">
          <div className="agent-activity-title">{t("questionRequest")}</div>
          <div className="agent-activity-subtitle">
            {pending ? t("questionWaitingForAnswer") : questionStatusLabel(question.status, t)}
          </div>
        </div>
        <span className={`agent-activity-badge status-${question.status}`} role="status">
          {questionStatusLabel(question.status, t)}
        </span>
      </div>
      {deadlineLabel ? (
        <div className="agent-question-timeout">
          <Clock3 aria-hidden="true" size={14} />
          <span>
            {pending
              ? t("questionDeadline", { time: deadlineLabel })
              : t("questionHadDeadline", { time: deadlineLabel })}
          </span>
        </div>
      ) : null}
      <div className="agent-question-list">
        {question.questions.map((item, questionIndex) => {
          const answer = question.answers?.[item.id];
          return (
            <section key={item.id} className="agent-question-item">
              <div className="agent-question-heading">
                <span>{t("questionProgress", { current: questionIndex + 1, total: question.questions.length })}</span>
                <strong>{item.header}</strong>
                {item.is_secret ? <LockKeyhole aria-label={t("questionSecretAnswer")} size={14} /> : null}
              </div>
              <p className="agent-question-prompt">{item.question}</p>
              {pending ? (
                <div className="agent-question-options">
                  {questionOptions(item).map((option, optionIndex) => (
                    <button
                      key={`${optionIndex}:${option.label}`}
                      type="button"
                      onClick={() => onSelect?.(question.id, item.id, optionIndex + 1)}
                    >
                      <span>{optionIndex + 1}</span>
                      <span>
                        <strong>
                          {item.is_other && optionIndex === questionOptions(item).length - 1
                            ? t("questionOther")
                            : option.label}
                        </strong>
                        {option.description ? <small>{option.description}</small> : null}
                      </span>
                    </button>
                  ))}
                  {questionOptions(item).length === 0 ? (
                    <Button size="sm" variant="secondaryColor" onClick={() => onSelect?.(question.id, item.id)}>
                      {t("questionAnswer")}
                    </Button>
                  ) : null}
                </div>
              ) : answer ? (
                <div className="agent-question-resolution">
                  {answer.skipped
                    ? t("questionSkipped")
                    : answer.secret
                      ? t("questionSecretRecorded")
                      : answer.option_label || answer.text || t("questionAnswered")}
                </div>
              ) : null}
            </section>
          );
        })}
      </div>
      {pending ? (
        <Button
          className="agent-question-answer-button"
          size="sm"
          variant="primary"
          onClick={() => onSelect?.(question.id)}
        >
          {t("questionAnswer")}
        </Button>
      ) : null}
    </section>
  );
}

function questionStatusLabel(status: string, t: TranslateFn): string {
  const key = `questionStatus${status.charAt(0).toUpperCase()}${status.slice(1)}`;
  return t(key);
}

function ToolActivityCard({ activity }: AgentActivityCardProps) {
  const tool = activity.content.tool!;
  const summary = tool.output_summary || tool.input_summary || activity.content.body || "";
  const kind = displayToolKind(tool.kind);
  const markdown = summary ? `🔧 \`${kind}\`\n\`\`\`\n${summary}\n\`\`\`` : `🔧 \`${kind}\``;
  return <div className="message-content" dangerouslySetInnerHTML={{ __html: renderMarkdown(markdown) }} />;
}

function PermissionActivityCard({ action, activity }: { action: AgentActivityAction; activity: AgentActivityPayload }) {
  const [busyOption, setBusyOption] = useState("");
  const [localStatus, setLocalStatus] = useState(action.status);
  const [error, setError] = useState("");
  const isPending = localStatus === "pending";
  const icon = useMemo(() => permissionIcon(localStatus), [localStatus]);

  useEffect(() => {
    setLocalStatus(action.status);
  }, [action.status]);

  async function decide(option: AgentActivityActionOption) {
    if (!action.id || busyOption || !isPending) {
      return;
    }
    setBusyOption(option.id);
    setError("");
    try {
      const snapshot = await decideChannelActivity(activity.channel, action.id, option.id);
      setLocalStatus(snapshot.status || optionStatus(option));
    } catch (err) {
      setError(errorMessage(err, "Permission decision failed"));
    } finally {
      setBusyOption("");
    }
  }

  return (
    <section className={`agent-activity-card agent-activity-permission status-${localStatus}`}>
      <div className="agent-activity-header">
        <span className="agent-activity-icon" aria-hidden="true">
          {icon}
        </span>
        <div className="agent-activity-title-group">
          <div className="agent-activity-title">{action.title}</div>
          <div className="agent-activity-subtitle">Permission request</div>
        </div>
        <span className={`agent-activity-badge status-${localStatus}`}>{statusLabel(localStatus)}</span>
      </div>
      {isPending ? (
        <div className="agent-activity-actions">
          {(action.options || []).map((option) => (
            <Button
              key={option.id}
              className={option.kind === "allow_always" ? "agent-activity-allow-always" : ""}
              disabled={Boolean(busyOption)}
              size="sm"
              variant={option.kind.startsWith("reject") ? "outlineDanger" : "secondaryColor"}
              onClick={() => void decide(option)}
            >
              <span className="agent-activity-button-icon" aria-hidden="true">
                {option.kind.startsWith("reject") ? <X size={14} /> : <Check size={14} />}
              </span>
              <span>{busyOption === option.id ? "..." : actionOptionLabel(option)}</span>
            </Button>
          ))}
        </div>
      ) : null}
      {error ? <div className="agent-activity-error">{error}</div> : null}
    </section>
  );
}

function NoticeActivityCard({ body }: { body: string }) {
  return (
    <section className="agent-activity-card agent-activity-notice">
      <div className="agent-activity-header">
        <span className="agent-activity-icon" aria-hidden="true">
          <ShieldAlert size={16} />
        </span>
        <div className="agent-activity-title-group">
          <div className="agent-activity-title">Agent notice</div>
          <div className="agent-activity-subtitle">{body}</div>
        </div>
      </div>
    </section>
  );
}

function permissionIcon(status: string) {
  if (status === "allowed") {
    return <ShieldCheck size={16} />;
  }
  if (status === "rejected" || status === "expired" || status === "canceled") {
    return <ShieldX size={16} />;
  }
  return <ShieldAlert size={16} />;
}

function optionStatus(option: AgentActivityActionOption): string {
  return option.kind.startsWith("reject") ? "rejected" : "allowed";
}

function displayToolKind(kind: string | undefined) {
  const normalized = String(kind || "")
    .trim()
    .toLowerCase();
  if (normalized === "execute") {
    return "exec";
  }
  return normalized || "tool";
}

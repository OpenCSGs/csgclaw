import messageContentStyles from "./MessageContent.module.css";
import { useEffect, useMemo, useState } from "react";
import { Check, ShieldAlert, ShieldCheck, ShieldX, X } from "lucide-react";
import { decideChannelActivity } from "@/api/agentActivities";
import { errorMessage } from "@/api/client";
import { AgentActivityMsgTypes } from "@/shared/constants/messages";
import { createTranslator } from "@/shared/i18n";
import { questionOptions } from "@/models/agentActivity";
import { StandaloneToolStep } from "@/components/business/TurnProgress";
import { isExecutionToolKind } from "@/models/turnProgress";
import type {
  AgentActivityAction,
  AgentActivityActionOption,
  AgentActivityPayload,
  AgentActivityQuestion,
} from "@/models/agentActivity";
import type { TranslateFn } from "@/models/conversations";
import { Button, QuestionCard } from "@/components/ui";

type AgentActivityCardProps = {
  activity: AgentActivityPayload;
  onQuestionSelect?: (activityID: string, questionID?: string, optionIndex?: number) => void;
  t?: TranslateFn;
};

export function AgentActivityCard({ activity, onQuestionSelect, t }: AgentActivityCardProps) {
  const translate = t ?? createTranslator("en");
  if (activity.content.msgtype === AgentActivityMsgTypes.tool && activity.content.tool) {
    return <ToolActivityCard activity={activity} t={translate} />;
  }
  if (activity.content.msgtype === AgentActivityMsgTypes.action && activity.content.action?.kind === "permission") {
    return <PermissionActivityCard activity={activity} action={activity.content.action} t={translate} />;
  }
  if (activity.content.msgtype === AgentActivityMsgTypes.question && activity.content.question) {
    return <QuestionActivityCard question={activity.content.question} onSelect={onQuestionSelect} t={translate} />;
  }
  return <NoticeActivityCard body={activity.content.body} t={translate} />;
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
    <QuestionCard
      key={question.id}
      title={t("questionRequest")}
      status={questionStatusLabel(question.status, t)}
      deadline={
        deadlineLabel ? t(pending ? "questionDeadline" : "questionHadDeadline", { time: deadlineLabel }) : undefined
      }
      questions={question.questions.map((item) => {
        const answer = question.answers?.[item.id];
        return {
          id: item.id,
          heading: item.header,
          prompt: item.question,
          privateAnswerLabel: item.is_secret ? t("questionSecretAnswer") : undefined,
          options: pending ? questionOptions(item) : [],
          resolution: answer
            ? answer.skipped
              ? t("questionSkipped")
              : answer.secret || item.is_secret
                ? t("questionSecretRecorded")
                : answer.option_label || answer.text || t("questionAnswered")
            : undefined,
        };
      })}
      labels={{
        answer: t("questionAnswer"),
        navigation: t("questionNavigation"),
        previous: t("questionPrevious"),
        next: t("questionNext"),
        progress: (current, total) => t("questionProgressCompact", { current, total }),
      }}
      onAnswer={
        pending && onSelect
          ? (id, index) => onSelect(question.id, id, index === undefined ? undefined : index + 1)
          : undefined
      }
    />
  );
}

function questionStatusLabel(status: string, t: TranslateFn): string {
  const key = `questionStatus${status.charAt(0).toUpperCase()}${status.slice(1)}`;
  return t(key);
}

function ToolActivityCard({ activity, t }: { activity: AgentActivityPayload; t: TranslateFn }) {
  const tool = activity.content.tool!;
  return (
    <StandaloneToolStep
      tool={{
        name: tool.title,
        actions: tool.kind ? [isExecutionToolKind(tool.kind) ? "execute" : tool.kind] : [],
        status: tool.status,
        summary: tool.progress_summary,
        command: tool.command,
        cwd: tool.cwd,
        input: tool.input_summary || activity.content.body,
        output: tool.output_summary,
        duration_ms: tool.duration_ms,
        exit_code: tool.exit_code ?? undefined,
      }}
      t={t}
    />
  );
}

function PermissionActivityCard({
  action,
  activity,
  t,
}: {
  action: AgentActivityAction;
  activity: AgentActivityPayload;
  t: TranslateFn;
}) {
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
      setError(errorMessage(err, t("permissionDecisionFailed")));
    } finally {
      setBusyOption("");
    }
  }

  return (
    <section className={`agent-activity-card agent-activity-permission status-${localStatus}`}>
      <div className={messageContentStyles["agent-activity-header"]}>
        <span className={messageContentStyles["agent-activity-icon"]} aria-hidden="true">
          {icon}
        </span>
        <div className={messageContentStyles["agent-activity-title-group"]}>
          <div className="agent-activity-title">{action.title}</div>
          <div className={messageContentStyles["agent-activity-subtitle"]}>{t("permissionRequest")}</div>
        </div>
        <span className={`${messageContentStyles["agent-activity-badge"]} status-${localStatus}`}>
          {permissionStatusLabel(localStatus, t)}
        </span>
      </div>
      {isPending ? (
        <div className={messageContentStyles["agent-activity-actions"]}>
          {(action.options || []).map((option) => (
            <Button
              key={option.id}
              className={option.kind === "allow_always" ? messageContentStyles["agent-activity-allow-always"] : ""}
              disabled={Boolean(busyOption)}
              size="sm"
              variant={option.kind.startsWith("reject") ? "outlineDanger" : "secondaryColor"}
              onClick={() => void decide(option)}
            >
              <span className={messageContentStyles["agent-activity-button-icon"]} aria-hidden="true">
                {option.kind.startsWith("reject") ? <X size={14} /> : <Check size={14} />}
              </span>
              <span>{busyOption === option.id ? "..." : permissionActionOptionLabel(option, t)}</span>
            </Button>
          ))}
        </div>
      ) : null}
      {error ? <div className={messageContentStyles["agent-activity-error"]}>{error}</div> : null}
    </section>
  );
}

function NoticeActivityCard({ body, t }: { body: string; t: TranslateFn }) {
  return (
    <section className="agent-activity-card agent-activity-notice">
      <div className={messageContentStyles["agent-activity-header"]}>
        <span className={messageContentStyles["agent-activity-icon"]} aria-hidden="true">
          <ShieldAlert size={16} />
        </span>
        <div className={messageContentStyles["agent-activity-title-group"]}>
          <div className="agent-activity-title">{t("agentNoticeTitle")}</div>
          <div className={messageContentStyles["agent-activity-subtitle"]}>{body}</div>
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

function permissionStatusLabel(status: string, t: TranslateFn): string {
  switch (status) {
    case "allowed":
      return t("permissionStatusAllowed");
    case "rejected":
      return t("permissionStatusRejected");
    case "expired":
      return t("permissionStatusExpired");
    case "canceled":
      return t("permissionStatusCanceled");
    case "completed":
      return t("permissionStatusCompleted");
    case "failed":
      return t("permissionStatusFailed");
    case "running":
      return t("permissionStatusRunning");
    case "pending":
      return t("permissionStatusPending");
    default:
      return t("permissionStatusUnknown");
  }
}

function permissionActionOptionLabel(option: AgentActivityActionOption, t: TranslateFn): string {
  switch (option.kind) {
    case "allow_once":
      return t("permissionAllowOnce");
    case "allow_always":
      return option.scope === "agent" || !option.scope ? t("permissionAllowAlwaysAgent") : t("permissionAllowAlways");
    case "reject_once":
    case "reject":
      return t("permissionReject");
    default:
      return option.label || option.id || t("permissionAction");
  }
}

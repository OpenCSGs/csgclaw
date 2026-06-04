import { useEffect, useMemo, useState } from "react";
import { Check, ShieldAlert, ShieldCheck, ShieldX, X } from "lucide-react";
import { decideChannelActivity } from "@/api/agentActivities";
import { errorMessage } from "@/api/client";
import { AgentActivityMsgTypes } from "@/shared/constants/messages";
import { actionOptionLabel, statusLabel } from "@/models/agentActivity";
import { renderMarkdown } from "./markdown";
import type { AgentActivityAction, AgentActivityActionOption, AgentActivityPayload } from "@/models/agentActivity";
import { Button } from "@/components/ui";

type AgentActivityCardProps = {
  activity: AgentActivityPayload;
};

export function AgentActivityCard({ activity }: AgentActivityCardProps) {
  if (activity.content.msgtype === AgentActivityMsgTypes.tool && activity.content.tool) {
    return <ToolActivityCard activity={activity} />;
  }
  if (activity.content.msgtype === AgentActivityMsgTypes.action && activity.content.action?.kind === "permission") {
    return <PermissionActivityCard activity={activity} action={activity.content.action} />;
  }
  return <NoticeActivityCard body={activity.content.body} />;
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

import { Button } from "@/components/ui";
import { StructuredMessageTitleBlock } from "./StructuredMessageCard";
import type { ActionCardPayload, MessageActionError, MessageLike } from "./types";

export type ActionCardProps = {
  busyKey?: string;
  data: ActionCardPayload;
  error?: MessageActionError | null;
  message?: MessageLike | null;
  onAction?: ActionCardPayload["actions"][number] extends infer Action
    ? (action: Action, message?: MessageLike | null) => void
    : never;
};

export function ActionCard({ data, message, busyKey, error, onAction }: ActionCardProps) {
  const actionError = data.actions?.some((action) => `${message?.id || "message"}:${action.id}` === error?.key)
    ? error?.message
    : "";

  return (
    <div className="structured-message action-card">
      <StructuredMessageTitleBlock data={data} />
      {data.summary ? <div className="structured-message-summary">{data.summary}</div> : null}
      {data.actions?.length ? (
        <div className="structured-message-actions">
          {data.actions.map((action) => {
            const key = `${message?.id || "message"}:${action.id}`;
            const busy = busyKey === key;
            const danger = action.style === "danger";
            return (
              <Button
                key={action.id}
                variant={danger ? "outlineDanger" : "secondaryGray"}
                className="structured-message-action-button"
                disabled={busy || !onAction}
                onClick={() => onAction?.(action, message)}
              >
                {busy ? "..." : action.label}
              </Button>
            );
          })}
        </div>
      ) : null}
      {actionError ? <div className="structured-message-action-error">{actionError}</div> : null}
      {data.fallback ? <div className="structured-message-subtitle">{data.fallback}</div> : null}
    </div>
  );
}

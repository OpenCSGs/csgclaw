import type { MessageTimestampParts } from "@/models/conversations";

export function MessageTimestamp({ parts }: { parts: MessageTimestampParts }) {
  if (!parts.shortLabel) {
    return null;
  }
  return (
    <time
      className="message-timestamp"
      dateTime={parts.dateTime}
      aria-label={parts.tooltip}
      data-tooltip={parts.tooltip}
      data-tooltip-side="top"
      tabIndex={0}
    >
      {parts.shortLabel}
    </time>
  );
}

export function MessageTimeDivider({ parts }: { parts: MessageTimestampParts }) {
  if (!parts.dividerLabel) {
    return null;
  }
  return (
    <div className="message-time-divider">
      <time
        className="message-time-divider-label"
        dateTime={parts.dateTime}
        data-tooltip={parts.tooltip}
        data-tooltip-side="top"
        tabIndex={0}
      >
        {parts.dividerLabel}
      </time>
    </div>
  );
}

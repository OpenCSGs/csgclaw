import type { ComponentProps } from "react";
import { classNames, legacyClassName } from "../../lib/classNames.js";
import styles from "./StatusBadge.module.css";

type StatusBadgeProps = ComponentProps<"span"> & {
  kind?: "status" | "tag" | "compact";
  tone?: "neutral" | "positive" | "warning";
};

const badgeClass = { status: "status-pill", tag: "mini-badge", compact: "agent-badge" };

export function StatusBadge({ kind = "status", tone = "neutral", className, ...props }: StatusBadgeProps) {
  return (
    <span
      {...props}
      data-kind={kind}
      data-tone={tone}
      className={classNames(legacyClassName(badgeClass[kind]), styles.badge, className)}
    />
  );
}

export function StatusDot({ online = false, className }: { online?: boolean; className?: string }) {
  return (
    <span
      aria-hidden="true"
      data-online={online}
      className={classNames(
        legacyClassName("workspace-status-dot"),
        styles.dot,
        online && legacyClassName("online"),
        className,
      )}
    />
  );
}

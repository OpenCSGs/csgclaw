import type { ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";

export function AgentProfileHeader({
  avatar,
  title,
  description,
  actions,
  className,
}: {
  avatar: ReactNode;
  title: ReactNode;
  description: ReactNode;
  actions: ReactNode;
  className?: string;
}) {
  return (
    <header className={classNames("entity-header", className)}>
      {avatar}
      <div className="entity-heading">
        <div className="entity-title-row">{title}</div>
        {description}
      </div>
      <div className="entity-toolbar">{actions}</div>
    </header>
  );
}

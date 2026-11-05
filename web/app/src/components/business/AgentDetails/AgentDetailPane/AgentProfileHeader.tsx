import type { ReactNode } from "react";

export function AgentProfileHeader({
  avatar,
  title,
  description,
  actions,
}: {
  avatar: ReactNode;
  title: ReactNode;
  description: ReactNode;
  actions: ReactNode;
}) {
  return (
    <header className="entity-header">
      {avatar}
      <div className="entity-heading">
        <div className="entity-title-row">{title}</div>
        {description}
      </div>
      <div className="entity-toolbar">{actions}</div>
    </header>
  );
}

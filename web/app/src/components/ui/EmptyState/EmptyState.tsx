import type { HTMLAttributes, ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";

export type EmptyStateProps = Omit<HTMLAttributes<HTMLDivElement>, "title"> & {
  title: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  actions?: ReactNode;
};

export function EmptyState({ title, description, icon, actions, className, ...props }: EmptyStateProps) {
  return (
    <div role="status" className={classNames("csg-empty-state", className)} {...props}>
      {icon ? <span aria-hidden="true">{icon}</span> : null}
      <h2>{title}</h2>
      {description ? <p>{description}</p> : null}
      {actions ? <div className="csg-empty-state-actions">{actions}</div> : null}
    </div>
  );
}

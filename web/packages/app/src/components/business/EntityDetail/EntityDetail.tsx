import type { ComponentProps, ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";
import styles from "./EntityDetail.module.css";

export function EntityPane({ className, ...props }: ComponentProps<"section">) {
  return <section {...props} className={classNames("entity-pane", styles.pane, className)} />;
}

export function EntityHeader({
  as: Element = "header",
  avatar,
  title,
  children,
  actions,
  className,
  headingClassName,
  titleClassName,
}: {
  as?: "header" | "div";
  avatar: ReactNode;
  title: ReactNode;
  children?: ReactNode;
  actions?: ReactNode;
  className?: string;
  headingClassName?: string;
  titleClassName?: string;
}) {
  return (
    <Element className={classNames("entity-header", styles.header, className)}>
      {avatar}
      <div className={classNames("entity-heading", styles.heading, headingClassName)}>
        <div className={classNames("entity-title-row", styles.title, titleClassName)}>{title}</div>
        {children}
      </div>
      {actions}
    </Element>
  );
}

export function EntityAvatar({ className, ...props }: ComponentProps<"div">) {
  return <div {...props} className={classNames("entity-avatar", styles.avatar, className)} />;
}

export function EntityField({
  className,
  label,
  labelClassName,
  children,
  ...props
}: ComponentProps<"div"> & { label: ReactNode; labelClassName?: string }) {
  return (
    <div {...props} className={classNames("entity-field", styles.field, className)}>
      <span className={labelClassName}>{label}</span>
      {children}
    </div>
  );
}

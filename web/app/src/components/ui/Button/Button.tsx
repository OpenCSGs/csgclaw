import styles from "./Button.module.css";
import { forwardRef } from "react";
import type { AnchorHTMLAttributes, ButtonHTMLAttributes, ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";

export type ButtonVariant =
  | "primary"
  | "secondaryGray"
  | "secondaryColor"
  | "tertiaryGray"
  | "tertiaryColor"
  | "linkGray"
  | "linkColor"
  | "danger"
  | "outlineDanger"
  | "tertiaryDanger"
  | "linkDanger"
  | "ghost";

export type ButtonSize = "sm" | "md" | "lg" | "xl" | "2xl";

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  active?: boolean;
  iconOnly?: boolean;
  loading?: boolean;
  loadingLabel?: ReactNode;
  size?: ButtonSize;
  variant?: ButtonVariant;
};

const variantClassNames: Record<ButtonVariant, string> = {
  primary: classNames(styles["btn-primary"], "btn-primary"),
  secondaryGray: classNames(styles["btn-secondary-gray"], "btn-secondary-gray"),
  secondaryColor: styles["btn-secondary-color"],
  tertiaryGray: styles["btn-tertiary-gray"],
  tertiaryColor: styles["btn-tertiary-color"],
  linkGray: styles["btn-link-gray"],
  linkColor: styles["btn-link-color"],
  danger: classNames(styles["btn-danger"], "btn-danger"),
  outlineDanger: styles["btn-outline-danger"],
  tertiaryDanger: styles["btn-tertiary-danger"],
  linkDanger: styles["btn-link-danger"],
  ghost: styles["btn-tertiary-gray"],
};

const sizeClassNames: Record<ButtonSize, string> = {
  sm: styles["btn-sm"],
  md: styles["btn-md"],
  lg: styles["btn-lg"],
  xl: styles["btn-xl"],
  "2xl": styles["btn-2xl"],
};

function buttonClassName(variant: ButtonVariant, size: ButtonSize, className?: string) {
  return classNames(styles["btn"], "btn", variantClassNames[variant], sizeClassNames[size], className);
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  {
    active,
    "aria-label": ariaLabel,
    children,
    className,
    disabled,
    iconOnly = false,
    loading = false,
    loadingLabel,
    size = "md",
    type = "button",
    variant = "secondaryGray",
    "aria-busy": ariaBusy,
    ...props
  },
  ref,
) {
  const loadingAccessibleLabel =
    typeof loadingLabel === "string" || typeof loadingLabel === "number" ? String(loadingLabel) : undefined;
  const childAccessibleLabel =
    typeof children === "string" || typeof children === "number" ? String(children) : undefined;

  return (
    <button
      ref={ref}
      type={type}
      disabled={disabled || loading}
      data-csg-control="button"
      className={buttonClassName(
        variant,
        size,
        classNames(
          active && classNames(styles["active"], "active"),
          iconOnly && styles["csg-icon-button"],
          loading && styles["btn-loading"],
          className,
        ),
      )}
      aria-label={ariaLabel ?? (loading ? loadingAccessibleLabel || childAccessibleLabel : undefined)}
      aria-busy={ariaBusy ?? (loading ? true : undefined)}
      {...props}
    >
      <span className={classNames(styles["btn-content"], "btn-content")}>{children}</span>
      {loading ? (
        <span className={styles["btn-loading-overlay"]} aria-hidden="true">
          <span className={styles["btn-loading-spinner"]} aria-hidden="true" />
        </span>
      ) : null}
    </button>
  );
});

export type LinkButtonProps = AnchorHTMLAttributes<HTMLAnchorElement> & Pick<ButtonProps, "size" | "variant">;

export const LinkButton = forwardRef<HTMLAnchorElement, LinkButtonProps>(function LinkButton(
  { children, className, size = "md", variant = "secondaryGray", ...props },
  ref,
) {
  return (
    <a ref={ref} data-csg-control="button" className={buttonClassName(variant, size, className)} {...props}>
      <span className={classNames(styles["btn-content"], "btn-content")}>{children}</span>
    </a>
  );
});

export type IconButtonProps = Omit<ButtonProps, "aria-label" | "children" | "iconOnly"> & {
  icon: ReactNode;
  label: string;
  markClassName?: string;
};

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { icon, label, markClassName, ...props },
  ref,
) {
  return (
    <Button ref={ref} iconOnly aria-label={label} {...props}>
      <span className={markClassName} aria-hidden="true">
        {icon}
      </span>
    </Button>
  );
});

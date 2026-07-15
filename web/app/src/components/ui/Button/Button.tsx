import { forwardRef } from "react";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { classNames } from "@/shared/lib/classNames";
import { Tooltip } from "../Tooltip";

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
  primary: "btn-primary",
  secondaryGray: "btn-secondary-gray",
  secondaryColor: "btn-secondary-color",
  tertiaryGray: "btn-tertiary-gray",
  tertiaryColor: "btn-tertiary-color",
  linkGray: "btn-link-gray",
  linkColor: "btn-link-color",
  danger: "btn-danger",
  outlineDanger: "btn-outline-danger",
  tertiaryDanger: "btn-tertiary-danger",
  linkDanger: "btn-link-danger",
  ghost: "btn-tertiary-gray",
};

const sizeClassNames: Record<ButtonSize, string> = {
  sm: "btn-sm",
  md: "btn-md",
  lg: "btn-lg",
  xl: "btn-xl",
  "2xl": "btn-2xl",
};

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
    title,
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

  const button = (
    <button
      ref={ref}
      type={type}
      disabled={disabled || loading}
      className={classNames(
        "btn",
        variantClassNames[variant],
        sizeClassNames[size],
        active && "active",
        iconOnly && "csg-icon-button",
        loading && "btn-loading",
        className,
      )}
      aria-label={ariaLabel ?? (loading ? loadingAccessibleLabel || childAccessibleLabel : undefined)}
      aria-busy={ariaBusy ?? (loading ? true : undefined)}
      {...props}
    >
      <span className="btn-content">{children}</span>
      {loading ? (
        <span className="btn-loading-overlay" aria-hidden="true">
          <span className="btn-loading-spinner" aria-hidden="true" />
        </span>
      ) : null}
    </button>
  );

  return title ? <Tooltip content={title}>{button}</Tooltip> : button;
});

export type IconButtonProps = Omit<ButtonProps, "aria-label" | "children" | "iconOnly"> & {
  icon: ReactNode;
  label: string;
  markClassName?: string;
};

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { icon, label, markClassName, title = label, ...props },
  ref,
) {
  return (
    <Button ref={ref} iconOnly aria-label={label} title={title} {...props}>
      <span className={markClassName} aria-hidden="true">
        {icon}
      </span>
    </Button>
  );
});

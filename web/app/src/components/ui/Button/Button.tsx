import { forwardRef } from "react";
import type { ButtonHTMLAttributes, ReactNode } from "react";
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
  { active, className, iconOnly = false, size = "md", type = "button", variant = "secondaryGray", ...props },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      className={classNames(
        "btn",
        variantClassNames[variant],
        sizeClassNames[size],
        active && "active",
        iconOnly && "csg-icon-button",
        className,
      )}
      {...props}
    />
  );
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

import { DismissibleAlert } from "../DismissibleAlert/index.js";
import type { ComponentProps, HTMLAttributes } from "react";
import { classNames, legacyClassName } from "../../lib/classNames.js";
import styles from "./InlineError.module.css";

type InlineErrorProps = HTMLAttributes<HTMLElement> & {
  as?: "div" | "p" | "span";
};

export function InlineError({ as: Element = "div", className, ...props }: InlineErrorProps) {
  return <Element {...props} className={classNames(legacyClassName("form-error"), styles.error, className)} />;
}

export function DismissibleInlineError({ className, ...props }: ComponentProps<typeof DismissibleAlert>) {
  return <DismissibleAlert {...props} className={classNames(legacyClassName("form-error"), styles.error, className)} />;
}

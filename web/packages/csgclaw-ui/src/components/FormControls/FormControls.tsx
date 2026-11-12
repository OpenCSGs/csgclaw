/*! Derived from CSGClaw. Modified for standalone csgclaw-ui distribution. */
import styles from "./FormControls.module.css";
import { forwardRef, useId } from "react";
import type { InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from "react";
import { classNames, legacyClassName } from "../../lib/classNames.js";
import { FieldContext, useFieldControlProps } from "./fieldContext.js";

export type FieldProps = {
  children: ReactNode;
  as?: "label" | "div";
  className?: string;
  error?: ReactNode;
  hint?: ReactNode;
  label?: ReactNode;
  labelAction?: ReactNode;
  required?: boolean;
  id?: string;
};

export function Field({
  children,
  as = "label",
  className,
  error,
  hint,
  label,
  labelAction,
  required = false,
  id,
}: FieldProps) {
  const generatedId = useId();
  const controlId = id ?? generatedId;
  const labelId = label ? `${controlId}-label` : undefined;
  const hintId = hint ? `${controlId}-hint` : undefined;
  const errorId = error ? `${controlId}-error` : undefined;
  const Root = labelAction ? "div" : as;
  const Label = Root === "div" ? "label" : "span";
  const fieldLabel = label ? (
    <Label
      className={classNames(styles["field-label"], legacyClassName("field-label"))}
      htmlFor={Root === "div" ? controlId : undefined}
      id={labelId}
    >
      {label}
      {required ? (
        <span
          className={classNames(styles["field-required"], legacyClassName("field-required"))}
          data-csg-field-part
          aria-hidden="true"
        >
          {" "}
          *
        </span>
      ) : null}
    </Label>
  ) : null;
  return (
    <FieldContext.Provider
      value={{
        controlId,
        labelId,
        descriptionIds: [hintId, errorId].filter(Boolean).join(" "),
        invalid: Boolean(error),
        required,
      }}
    >
      <Root
        className={classNames(
          classNames(styles["field"], legacyClassName("field")),
          classNames(styles["csg-field"], legacyClassName("csg-field")),
          className,
        )}
        data-csg-field
        data-invalid={error ? "true" : undefined}
      >
        {labelAction ? (
          <div className={styles["field-heading"]}>
            {fieldLabel}
            {labelAction}
          </div>
        ) : (
          fieldLabel
        )}
        {children}
        {hint ? (
          <small
            className={classNames(styles["field-hint"], legacyClassName("field-hint"))}
            data-csg-field-part
            id={hintId}
          >
            {hint}
          </small>
        ) : null}
        {error ? (
          <div
            className={classNames(styles["form-error"], legacyClassName("form-error"))}
            data-csg-field-part
            id={errorId}
            role="alert"
          >
            {error}
          </div>
        ) : null}
      </Root>
    </FieldContext.Provider>
  );
}

export type TextInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, "size"> & {
  size?: "sm" | "md";
  variant?: "default" | "plain";
};

export const TextInput = forwardRef<HTMLInputElement, TextInputProps>(function TextInput(
  { className, size = "md", variant = "default", ...props },
  ref,
) {
  const controlProps = useFieldControlProps(props);
  return (
    <input
      ref={ref}
      className={classNames(styles["csg-text-control"], className)}
      data-csg-control="input"
      data-size={size}
      data-variant={variant}
      {...controlProps}
    />
  );
});

export type TextAreaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

export const TextArea = forwardRef<HTMLTextAreaElement, TextAreaProps>(function TextArea({ className, ...props }, ref) {
  const controlProps = useFieldControlProps(props);
  return (
    <textarea
      ref={ref}
      className={classNames(styles["csg-text-control"], className)}
      data-csg-control="textarea"
      {...controlProps}
    />
  );
});

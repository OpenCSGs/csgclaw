import { forwardRef, useId } from "react";
import type { InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from "react";
import { classNames } from "@/shared/lib/classNames";
import { FieldContext, useFieldControlProps } from "./fieldContext";

export type FieldProps = {
  children: ReactNode;
  className?: string;
  error?: ReactNode;
  hint?: ReactNode;
  label?: ReactNode;
  required?: boolean;
  id?: string;
};

export function Field({ children, className, error, hint, label, required = false, id }: FieldProps) {
  const generatedId = useId();
  const controlId = id ?? generatedId;
  const labelId = label ? `${controlId}-label` : undefined;
  const hintId = hint ? `${controlId}-hint` : undefined;
  const errorId = error ? `${controlId}-error` : undefined;
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
      <label className={classNames("field", "csg-field", className)} data-invalid={error ? "true" : undefined}>
        {label ? (
          <span className="field-label" id={labelId}>
            {label}
            {required ? (
              <span className="field-required" aria-hidden="true">
                {" "}
                *
              </span>
            ) : null}
          </span>
        ) : null}
        {children}
        {hint ? (
          <small className="field-hint" id={hintId}>
            {hint}
          </small>
        ) : null}
        {error ? (
          <div className="form-error" id={errorId} role="alert">
            {error}
          </div>
        ) : null}
      </label>
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
      className={classNames("csg-text-control", className)}
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
      className={classNames("csg-text-control", className)}
      data-csg-control="textarea"
      {...controlProps}
    />
  );
});

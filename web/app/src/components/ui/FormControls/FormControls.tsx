import { forwardRef } from "react";
import type { InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from "react";
import { classNames } from "@/shared/lib/classNames";

export type FieldProps = {
  children: ReactNode;
  className?: string;
  error?: ReactNode;
  hint?: ReactNode;
  label?: ReactNode;
};

export function Field({ children, className, error, hint, label }: FieldProps) {
  return (
    <label className={classNames("field", className)} data-invalid={error ? "true" : undefined}>
      {label ? <span className="field-label">{label}</span> : null}
      {children}
      {hint ? <small className="field-hint">{hint}</small> : null}
      {error ? <div className="form-error">{error}</div> : null}
    </label>
  );
}

export type TextInputProps = InputHTMLAttributes<HTMLInputElement>;

export const TextInput = forwardRef<HTMLInputElement, TextInputProps>(function TextInput(props, ref) {
  return <input ref={ref} {...props} />;
});

export type TextAreaProps = TextareaHTMLAttributes<HTMLTextAreaElement>;

export const TextArea = forwardRef<HTMLTextAreaElement, TextAreaProps>(function TextArea(props, ref) {
  return <textarea ref={ref} {...props} />;
});

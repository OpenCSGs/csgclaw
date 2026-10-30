import { useLabTranslation } from "../LabLocale";
import type { ChangeEventHandler } from "react";
import { Field, TextArea, TextInput } from "@/components/ui";
import mail from "./assets/16aa9.svg";
import disabledMailMd from "./assets/9cb91.svg";
import disabledMailSm from "./assets/8b6af.svg";
import help from "./assets/e9c84.svg";
import alert from "./assets/37d81.svg";
import resize from "./assets/45905.svg";
import styles from "./InputFoundations.module.css";

type InputSampleProps = {
  size?: "sm" | "md";
  multiline?: boolean;
  state?: string;
  error?: string;
  value: string;
  onChange?: ChangeEventHandler<HTMLInputElement | HTMLTextAreaElement>;
  reference?: boolean;
};

export function InputSample({
  size = "md",
  multiline = false,
  state = "Default",
  error,
  value,
  onChange,
  reference = false,
}: InputSampleProps) {
  const { text: labText } = useLabTranslation();
  const sampleText = reference ? (source: string) => source : labText;
  const disabled = state === "Disabled";
  const placeholder = multiline ? sampleText("Enter a description...") : "olivia@untitledui.com";
  return (
    <div className={styles.sample} data-size={size} data-state={reference ? state : undefined} data-error={!!error}>
      <Field
        className={styles.field}
        label={
          <>
            {sampleText(multiline ? "Description" : "Email")}
            {multiline && <img className={styles.labelHelp} src={help} alt="" />}
          </>
        }
        required
        hint={error ? undefined : sampleText("This is a hint text to help user.")}
        error={error}
      >
        {multiline ? (
          <div className={styles.textareaWrap}>
            <TextArea
              className={styles.textarea}
              value={value}
              onChange={onChange}
              readOnly={reference}
              disabled={disabled}
              placeholder={placeholder}
              required
            />
            <img className={styles.resize} src={resize} alt="" />
          </div>
        ) : (
          <div className={styles.inputBox} data-disabled={disabled}>
            <img src={disabled ? (size === "sm" ? disabledMailSm : disabledMailMd) : mail} alt="" />
            <TextInput
              className={styles.input}
              variant="plain"
              value={value}
              onChange={onChange}
              readOnly={reference}
              disabled={disabled}
              type="email"
              placeholder={placeholder}
              required
            />
            <img src={error ? alert : help} alt="" />
          </div>
        )}
      </Field>
    </div>
  );
}

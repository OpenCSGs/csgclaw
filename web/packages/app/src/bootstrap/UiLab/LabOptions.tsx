import { useId, type ReactNode } from "react";
import { Select, type SelectProps } from "@/components/ui";
import styles from "./LabOptions.module.css";

export function LabOptions({ children }: { children: ReactNode }) {
  return <div className={styles.row}>{children}</div>;
}

export function LabOptionSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: SelectProps["options"];
}) {
  const labelId = useId();
  return (
    <div className={styles.field}>
      <span id={labelId}>{label}</span>
      <Select
        size="sm"
        value={value}
        onValueChange={onChange}
        options={options}
        triggerClassName={styles.trigger}
        triggerProps={{ "aria-labelledby": labelId }}
      />
    </div>
  );
}

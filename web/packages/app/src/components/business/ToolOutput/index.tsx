import styles from "./ToolOutput.module.css";

export function ToolOutput({ label, text }: { label: string; text: string }) {
  return (
    <div className={styles.output} role="region" aria-label={label} tabIndex={0}>
      <pre>{text}</pre>
    </div>
  );
}

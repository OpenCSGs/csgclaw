import styles from "./FloatingChatGuide.module.css";

export type FloatingChatGuideProps = {
  dismissLabel: string;
  title: string;
  onDismiss: () => void;
  onOpen: () => void;
};

export function FloatingChatGuide({ dismissLabel, title, onDismiss, onOpen }: FloatingChatGuideProps) {
  return (
    <div className={styles.root} aria-live="polite">
      <button type="button" className={styles.callout} aria-label={title} onClick={onOpen}>
        <span className={styles.title}>{title}</span>
        <span className={styles.trail} aria-hidden="true" />
      </button>
      <button
        type="button"
        className={styles.dismiss}
        aria-label={dismissLabel}
        title={dismissLabel}
        onClick={onDismiss}
      >
        <span aria-hidden="true" />
      </button>
    </div>
  );
}

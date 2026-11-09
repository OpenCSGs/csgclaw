import styles from "./FileTypeIcon.module.css";

export function FolderIcon({ size = 24 }: { size?: number }) {
  return <span className={styles.folderIcon} style={{ width: size, height: size }} aria-hidden="true" />;
}

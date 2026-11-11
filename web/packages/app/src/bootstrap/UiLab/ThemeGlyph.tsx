import type { CSSProperties } from "react";
import styles from "./ThemeGlyph.module.css";

export function ThemeGlyph({ src, className }: { src: string; className?: string }) {
  return (
    <span
      className={className ? `${styles.glyph} ${className}` : styles.glyph}
      style={{ "--lab-glyph-mask": `url("${src}")` } as CSSProperties}
      aria-hidden="true"
    >
      <img src={src} alt="" />
    </span>
  );
}

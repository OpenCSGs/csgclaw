import { classNames } from "@/shared/lib/classNames";
import styles from "./WorkspaceSidebar.module.css";

export function LogoWordmark() {
  return (
    <span className={styles.logo} aria-label="CSGClaw">
      <img
        className={classNames(styles.logoImage, styles.logoLight)}
        src="brand/csgclaw-logo-light.svg"
        alt="CSGClaw"
      />
      <img
        className={classNames(styles.logoImage, styles.logoDark)}
        src="brand/csgclaw-logo-dark.svg"
        alt=""
        aria-hidden="true"
      />
    </span>
  );
}

export function LogoMark() {
  return (
    <>
      <img
        className={classNames(styles.logoMarkImage, styles.logoMarkLight)}
        src="brand/csgclaw-logo-collapsed.svg"
        alt=""
        aria-hidden="true"
      />
      <img
        className={classNames(styles.logoMarkImage, styles.logoMarkDark)}
        src="brand/csgclaw-logo-collapsed-dark.svg"
        alt=""
        aria-hidden="true"
      />
    </>
  );
}

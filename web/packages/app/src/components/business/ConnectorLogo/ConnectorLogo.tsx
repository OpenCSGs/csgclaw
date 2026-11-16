import styles from "./ConnectorLogo.module.css";
import type { ReactNode } from "react";
import { connectorLogoSource } from "@/shared/connectorLogo";

export function ConnectorLogo({
  name,
  className,
  fallback,
}: {
  name: string;
  className?: string;
  fallback: ReactNode;
}) {
  const src = connectorLogoSource(name);
  if (!src) return fallback;
  const logoClassName = className ? `${styles.logo} ${className}` : styles.logo;
  return <img className={logoClassName} src={src} width={18} height={18} alt="" aria-hidden="true" />;
}

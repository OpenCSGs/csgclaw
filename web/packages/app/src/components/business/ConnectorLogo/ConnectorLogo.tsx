import styles from "./ConnectorLogo.module.css";
import type { ReactNode } from "react";
import { ConnectorGitLabIcon } from "@/components/business/Icons";

type ConnectorBrand = "feishu" | "github" | "gitlab";

function connectorBrand(value: string): ConnectorBrand | undefined {
  const name = value.trim().toLowerCase();
  if (name.startsWith("飞书") || /(?:^|[^a-z0-9])(?:feishu|lark)(?:$|[^a-z0-9])/u.test(name)) return "feishu";
  if (/(?:^|[^a-z0-9])github(?:$|[^a-z0-9])/u.test(name)) return "github";
  if (/(?:^|[^a-z0-9])gitlab(?:$|[^a-z0-9])/u.test(name)) return "gitlab";
  return undefined;
}

export function ConnectorLogo({
  name,
  className,
  fallback,
}: {
  name: string;
  className?: string;
  fallback: ReactNode;
}) {
  const brand = connectorBrand(name);
  if (!brand) return fallback;
  const logoClassName = className ? `${styles.logo} ${className}` : styles.logo;
  if (brand === "gitlab") return <ConnectorGitLabIcon size={18} className={logoClassName} aria-hidden="true" />;
  const src = brand === "feishu" ? "icons/feishu.png" : "icons/github.svg";
  return <img className={logoClassName} src={src} width={18} height={18} alt="" aria-hidden="true" />;
}

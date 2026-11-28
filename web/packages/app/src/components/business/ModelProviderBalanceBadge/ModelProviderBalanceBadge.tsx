import { Wallet } from "lucide-react";
import type { LocaleCode, TranslateFn } from "@/models/conversations";
import {
  modelProviderBalanceBadgeText,
  modelProviderBalanceDetailLines,
  modelProviderBalanceTone,
  type ModelProviderBalance,
} from "@/models/modelProviderBalance";
import { classNames } from "@/shared/lib/classNames";
import styles from "./ModelProviderBalanceBadge.module.css";

type ModelProviderBalanceBadgeProps = {
  balance?: ModelProviderBalance;
  compact?: boolean;
  locale?: LocaleCode;
  t: TranslateFn;
  className?: string;
};

export function ModelProviderBalanceBadge({
  balance,
  compact = false,
  locale = typeof document === "undefined" ? "zh" : document.documentElement.lang,
  t,
  className,
}: ModelProviderBalanceBadgeProps) {
  const text = modelProviderBalanceBadgeText(balance, t, locale);
  if (!balance || !text) {
    return null;
  }
  const lines = modelProviderBalanceDetailLines(balance, t, locale);
  const tone = modelProviderBalanceTone(balance);
  return (
    <span
      className={classNames(styles.badge, className)}
      data-role="model-provider-balance"
      data-tone={tone}
      data-compact={compact ? "true" : undefined}
      title={lines.join("\n")}
    >
      {compact ? null : <Wallet size={12} aria-hidden="true" />}
      <span className={styles.value}>{text}</span>
    </span>
  );
}

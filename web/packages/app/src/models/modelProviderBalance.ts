import type { LocaleCode, TranslateFn } from "@/models/conversations";

export type ModelProviderBalanceKind = "balance" | "quota";

export type ModelProviderBalance = {
  kind: ModelProviderBalanceKind;
  remaining?: number;
  used?: number;
  total?: number;
  currency?: string;
  unit?: string;
  plan?: string;
  expires_at?: string;
  reset_at?: string;
  updated_at?: string;
};

export type ModelProviderBalanceTone = "ok" | "warning" | "danger";

export function normalizeModelProviderBalance(
  raw: unknown,
  fallbackKind: ModelProviderBalanceKind = "balance",
): ModelProviderBalance | undefined {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) {
    return undefined;
  }
  const record = raw as Record<string, unknown>;
  const total = numberOrUndefined(record.total ?? record.quota_total);
  const used = numberOrUndefined(record.used ?? record.quota_used);
  const currency = textOrUndefined(record.currency ?? record.currency_symbol);
  const kind = record.kind ?? fallbackKind;
  if (kind !== "balance" && kind !== "quota") return undefined;

  const remaining =
    numberOrUndefined(record.remaining ?? record.balance) ??
    (kind === "quota" && total !== undefined && used !== undefined ? Math.max(0, total - used) : undefined);
  const expires_at = textOrUndefined(record.expires_at);
  const reset_at = textOrUndefined(record.reset_at);
  if (remaining === undefined && (kind === "balance" || !(expires_at || reset_at))) return undefined;
  return {
    kind,
    remaining,
    used,
    total,
    currency,
    unit: textOrUndefined(record.unit),
    plan: textOrUndefined(record.plan ?? record.plan_name),
    expires_at,
    reset_at,
    updated_at: textOrUndefined(record.updated_at),
  };
}

export function modelProviderBalanceRatio(balance: ModelProviderBalance | undefined): number | null {
  if (!balance) {
    return null;
  }
  if (balance.kind === "quota" && balance.total !== undefined && balance.total > 0 && balance.remaining !== undefined) {
    return clamp(balance.remaining / balance.total);
  }
  if (balance.kind === "quota" && balance.total !== undefined && balance.total > 0 && balance.used !== undefined) {
    return clamp((balance.total - balance.used) / balance.total);
  }
  return null;
}

export function modelProviderBalanceTone(balance: ModelProviderBalance | undefined): ModelProviderBalanceTone {
  if (!balance) {
    return "ok";
  }
  const ratio = modelProviderBalanceRatio(balance);
  if (ratio !== null) {
    if (ratio <= 0.05) {
      return "danger";
    }
    return ratio <= 0.2 ? "warning" : "ok";
  }
  if (balance.kind === "balance" && balance.remaining !== undefined) {
    if (balance.remaining <= 0) {
      return "danger";
    }
    const currency = balance.currency?.toUpperCase();
    return (currency === "CNY" || currency === "USD" || currency === "¥") && balance.remaining < 10 ? "warning" : "ok";
  }
  return "ok";
}

export function modelProviderBalanceBadgeText(
  balance: ModelProviderBalance | undefined,
  t: TranslateFn,
  locale: LocaleCode,
): string {
  if (!balance) {
    return "";
  }
  if (balance.kind === "balance") {
    const amount = balance.remaining;
    if (amount === undefined) {
      return "";
    }
    return `${currencySymbol(balance.currency)}${formatCurrencyAmount(amount, locale)}`;
  }
  const ratio = modelProviderBalanceRatio(balance);
  if (ratio !== null) {
    return t("modelProviderBalanceRemainingPercent", { percent: Math.round(ratio * 100) });
  }
  if (balance.remaining !== undefined) {
    return t("modelProviderBalanceRemainingValue", {
      value: `${formatAmount(balance.remaining)}${balance.unit ? ` ${balance.unit}` : ""}`,
    });
  }
  if (balance.expires_at)
    return t("modelProviderBalanceExpires", { time: formatBalanceTime(balance.expires_at, locale) });
  return balance.reset_at ? t("modelProviderBalanceResets", { time: formatBalanceTime(balance.reset_at, locale) }) : "";
}

export function modelProviderBalanceDetailLines(
  balance: ModelProviderBalance | undefined,
  t: TranslateFn,
  locale: LocaleCode,
): string[] {
  if (!balance) {
    return [];
  }
  const lines: string[] = [];
  if (balance.kind === "balance") {
    if (balance.remaining !== undefined) {
      lines.push(
        t("modelProviderBalanceAccount", {
          value: `${currencySymbol(balance.currency)}${formatCurrencyAmount(balance.remaining, locale)}`,
        }),
      );
    }
  } else {
    if (balance.plan) {
      lines.push(t("modelProviderBalancePlan", { plan: balance.plan }));
    }
    const ratio = modelProviderBalanceRatio(balance);
    if (ratio !== null) {
      lines.push(t("modelProviderBalanceRemainingPercent", { percent: Math.round(ratio * 100) }));
    }
    if (balance.remaining !== undefined) {
      const remaining = `${formatAmount(balance.remaining)}${balance.unit ? ` ${balance.unit}` : ""}`;
      lines.push(
        balance.total !== undefined
          ? t("modelProviderBalanceQuotaOf", { remaining, total: formatAmount(balance.total) })
          : t("modelProviderBalanceRemainingValue", { value: remaining }),
      );
    }
    if (balance.reset_at) {
      lines.push(t("modelProviderBalanceResets", { time: formatBalanceTime(balance.reset_at, locale) }));
    }
    if (balance.expires_at) {
      lines.push(t("modelProviderBalanceExpires", { time: formatBalanceTime(balance.expires_at, locale) }));
    }
  }
  if (balance.updated_at) {
    lines.push(t("modelProviderBalanceUpdated", { time: formatBalanceTime(balance.updated_at, locale) }));
  }
  return lines;
}

function currencySymbol(currency: string | undefined): string {
  const raw = String(currency || "").trim();
  if (!raw) {
    return "";
  }
  const upper = raw.toUpperCase();
  if (upper === "CNY" || upper === "RMB" || raw === "￥" || raw === "¥") {
    return "¥";
  }
  if (upper === "USD") {
    return "$";
  }
  if (upper === "EUR") {
    return "€";
  }
  return /^[A-Z]{3}$/.test(upper) ? `${upper} ` : raw;
}

function formatCurrencyAmount(value: number, locale: LocaleCode): string {
  return new Intl.NumberFormat(locale === "en" ? "en-US" : "zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(value);
}

function formatAmount(value: number): string {
  if (!Number.isFinite(value)) {
    return "";
  }
  const abs = Math.abs(value);
  if (abs >= 10_000) {
    return new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 }).format(value);
  }
  if (Number.isInteger(value)) {
    return String(value);
  }
  return value.toFixed(2);
}

function formatBalanceTime(value: string, locale: LocaleCode): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  const intlLocale = locale === "en" ? "en-US" : "zh-CN";
  return new Intl.DateTimeFormat(intlLocale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(parsed);
}

function clamp(value: number): number {
  if (!Number.isFinite(value)) {
    return 0;
  }
  return Math.min(1, Math.max(0, value));
}

function numberOrUndefined(value: unknown): number | undefined {
  const parsed = typeof value === "string" && value.trim() ? Number(value) : value;
  return typeof parsed === "number" && Number.isFinite(parsed) ? parsed : undefined;
}

function textOrUndefined(value: unknown): string | undefined {
  return typeof value === "string" ? value.trim() || undefined : undefined;
}

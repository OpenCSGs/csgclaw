import { messages } from "@/shared/i18n/messages";
import { LOCALE_STORAGE_KEY } from "@/shared/storage/keys";
import type { LocaleCode, TranslateFn } from "@/models/conversations";

type ErrorTranslationKey = keyof typeof messages.zh.errors;

export function detectInitialLocale(): LocaleCode {
  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  if (stored === "zh" || stored === "en") {
    return stored;
  }
  return navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en";
}

export function createTranslator(locale: LocaleCode): TranslateFn {
  return (key, params = {}) => {
    const value = resolveTranslation(locale, key);
    if (typeof value !== "string") {
      return key;
    }
    return value.replace(/\{(\w+)\}/g, (_, name) => `${params[name] ?? ""}`);
  };
}

export function resolveTranslation(locale: LocaleCode, key: string): unknown {
  const catalog = locale === "zh" ? messages.zh : messages.en;
  return key.split(".").reduce<unknown>((current, part) => {
    if (!current || typeof current !== "object") {
      return undefined;
    }
    return (current as Record<string, unknown>)[part];
  }, catalog);
}

export function localizeRole(role: string, t: TranslateFn): string {
  return t(`roles.${role}`) === `roles.${role}` ? role : t(`roles.${role}`);
}

// Legacy contract: function localizeTemplateSourceTag(source, locale)
export function localizeTemplateSourceTag(source: unknown, locale: LocaleCode): string {
  const value = String(source ?? "").trim();
  if (!value) {
    return "-";
  }
  if (locale === "zh") {
    if (value === "builtin") {
      return "内建";
    }
    if (value === "local") {
      return "本地";
    }
    if (value === "official") {
      return "官方";
    }
    if (value === "personal") {
      return "个人";
    }
  }
  return value;
}

export function localizeError(raw: unknown, t: TranslateFn): string {
  const cleaned = String(raw ?? "").trim();
  const errorKeys = Object.keys(messages.zh.errors) as ErrorTranslationKey[];
  for (const key of errorKeys) {
    if (cleaned.includes(key)) {
      return t(`errors.${key}`);
    }
    const englishValue = messages.en.errors[key];
    if (englishValue && cleaned.includes(englishValue)) {
      return t(`errors.${key}`);
    }
    const prefix = `${key}:`;
    if (cleaned.startsWith(prefix)) {
      const suffix = cleaned.slice(prefix.length).trim();
      return `${t(`errors.${key}`)} ${suffix}`;
    }
  }
  return cleaned;
}

export function localizeRuntimeError(raw: unknown, t: TranslateFn): string {
  const value = String(raw ?? "").trim();
  if (!value.toLowerCase().startsWith("runtime error:")) {
    return value;
  }
  const localized = localizeError(value, t);
  const message = localized === value ? localizeRuntimeHTTPStatus(value, t) : localized;
  const billingURL = runtimeErrorBillingURL(value);
  return billingURL ? `${message}\n\n👉 [${t("rechargeAccount")}](${billingURL})` : message;
}

function localizeRuntimeHTTPStatus(value: string, t: TranslateFn): string {
  const match = value.match(/unexpected status\s+(\d{3})\b/i);
  const status = Number(match?.[1] ?? 0);
  if (status === 400 || status === 422) return t("errors.invalid_request_error");
  if (status === 401) return t("errors.authentication_error");
  if (status === 402) return t("errors.payment_required");
  if (status === 403) return t("errors.forbidden");
  if (status === 404) return t("errors.not_found");
  if (status === 429) return t("errors.rate_limit_exceeded");
  if (status >= 500 && status <= 599) return t("errors.upstream_unavailable");
  return t("errors.upstream_unavailable");
}

function runtimeErrorBillingURL(value: string): string {
  const jsonMatch = value.match(/"billing_url"\s*:\s*("(?:\\.|[^"\\])*")/i);
  if (jsonMatch) {
    try {
      return safeRuntimeErrorURL(JSON.parse(jsonMatch[1]));
    } catch (_) {
      return "";
    }
  }
  const markdownMatch = value.match(/\[Recharge your account\]\(([^)\s]+)\)/i);
  return markdownMatch ? safeRuntimeErrorURL(markdownMatch[1]) : "";
}

function safeRuntimeErrorURL(candidate: unknown): string {
  if (typeof candidate !== "string") return "";
  try {
    const parsed = new URL(candidate);
    return (parsed.protocol === "https:" || parsed.protocol === "http:") && !parsed.username && !parsed.password
      ? parsed.toString()
      : "";
  } catch (_) {
    return "";
  }
}

export function localizeAPIError(error: unknown, t: TranslateFn, fallback = ""): string {
  if (error && typeof error === "object" && "code" in error) {
    const code = String((error as { code?: unknown }).code || "").trim();
    if (code) {
      const key = `errors.${code}`;
      const localized = t(key);
      if (localized !== key) {
        return localized;
      }
    }
  }
  if (error && typeof error === "object" && "message" in error) {
    return localizeError((error as { message?: unknown }).message, t) || fallback;
  }
  return localizeError(error, t) || fallback;
}

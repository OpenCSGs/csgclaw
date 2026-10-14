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
  if (!cleaned) {
    return "";
  }
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

export function localizeAPIError(error: unknown, t: TranslateFn, fallback = ""): string {
  const defaultFallback = localizeErrorFallback(fallback, t) || defaultAPIError(t);
  if (error && typeof error === "object" && "code" in error) {
    const code = String((error as { code?: unknown }).code || "").trim();
    if (code) {
      const key = `errors.${code}`;
      const localized = t(key);
      const codeFallback = localized !== key ? localized : defaultFallback;
      if (code === "template_publish_failed" && "message" in error) {
        const message = localizeUnknownError((error as { message?: unknown }).message, t, codeFallback);
        if (message) {
          return message;
        }
      }
      if (localized !== key) {
        return localized;
      }
    }
  }
  if (error && typeof error === "object" && "message" in error) {
    return localizeUnknownError((error as { message?: unknown }).message, t, defaultFallback);
  }
  return localizeUnknownError(error, t, defaultFallback);
}

function localizeErrorFallback(fallback: unknown, t: TranslateFn): string {
  const localized = localizeError(fallback, t);
  if (isChineseTranslator(t) && isLikelyEnglishDiagnostic(localized)) {
    return defaultAPIError(t);
  }
  return localized;
}

function localizeUnknownError(raw: unknown, t: TranslateFn, fallback: string): string {
  const localized = localizeError(raw, t);
  if (!localized) {
    return fallback;
  }
  if (isChineseTranslator(t) && isLikelyEnglishDiagnostic(localized)) {
    return fallback;
  }
  return localized;
}

function defaultAPIError(t: TranslateFn): string {
  const key = "errors.unknown_api_error";
  const localized = t(key);
  return localized === key ? "" : localized;
}

function isChineseTranslator(t: TranslateFn): boolean {
  return t("errors.unknown_api_error") === messages.zh.errors.unknown_api_error;
}

function isLikelyEnglishDiagnostic(value: string): boolean {
  return /[A-Za-z]/.test(value) && !/[\u4e00-\u9fff]/.test(value);
}

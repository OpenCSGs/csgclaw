import { messages } from "@/shared/i18n/messages";
import { LOCALE_STORAGE_KEY } from "@/shared/storage/keys";
import type { LocaleCode, TranslateFn } from "@/models/conversations";

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
  }
  return value;
}

export function localizeError(raw: unknown, t: TranslateFn): string {
  const cleaned = String(raw ?? "").trim();
  for (const key of Object.keys(messages.zh.errors)) {
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

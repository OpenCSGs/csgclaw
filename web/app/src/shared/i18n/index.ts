// @ts-nocheck
import { messages } from "@/shared/i18n/messages";
import { LOCALE_STORAGE_KEY } from "@/shared/storage/keys";

export function detectInitialLocale() {
  const stored = window.localStorage.getItem(LOCALE_STORAGE_KEY);
  if (stored === "zh" || stored === "en") {
    return stored;
  }
  return navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en";
}

export function createTranslator(locale) {
  return (key, params = {}) => {
    const value = resolveTranslation(locale, key);
    if (typeof value !== "string") {
      return key;
    }
    return value.replace(/\{(\w+)\}/g, (_, name) => `${params[name] ?? ""}`);
  };
}

export function resolveTranslation(locale, key) {
  return key.split(".").reduce((current, part) => current?.[part], messages[locale]);
}

export function localizeRole(role, t) {
  return t(`roles.${role}`) === `roles.${role}` ? role : t(`roles.${role}`);
}

export function localizeTemplateSourceTag(source, locale) {
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

export function localizeError(raw, t) {
  const cleaned = raw.trim();
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

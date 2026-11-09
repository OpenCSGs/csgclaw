import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import type { LocaleCode } from "@/models/conversations";
import { UI_LAB_LOCALE_STORAGE_KEY } from "@/shared/storage/keys";
import { LabLocaleContext } from "./context";
import { createLabText } from "./translation";

function initialLocale(): LocaleCode {
  try {
    return localStorage.getItem(UI_LAB_LOCALE_STORAGE_KEY) === "en" ? "en" : "zh";
  } catch {
    return "zh";
  }
}
export function LabLocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<LocaleCode>(initialLocale);
  useEffect(() => {
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
    try {
      localStorage.setItem(UI_LAB_LOCALE_STORAGE_KEY, locale);
    } catch {
      /* Language switching still works without storage. */
    }
  }, [locale]);
  const value = useMemo(() => ({ locale, setLocale, text: createLabText(locale) }), [locale]);
  return <LabLocaleContext.Provider value={value}>{children}</LabLocaleContext.Provider>;
}

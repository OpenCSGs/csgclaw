import { createTranslator } from "@/shared/i18n";
import { labMessageKeys } from "@/shared/i18n/uiLab";
import type { LocaleCode } from "@/models/conversations";

export function createLabText(locale: LocaleCode) {
  const t = createTranslator(locale);
  return (source: string) => {
    const key = labMessageKeys[source];
    return key ? t(`uiLab.${key}`) : source;
  };
}

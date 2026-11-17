import { createContext, useContext } from "react";
import type { LocaleCode } from "@/models/conversations";
import { createLabText } from "./translation";

type LabLocaleValue = { locale: LocaleCode; setLocale: (locale: LocaleCode) => void; text: (source: string) => string };
export const LabLocaleContext = createContext<LabLocaleValue>({
  locale: "zh",
  setLocale: () => {},
  text: createLabText("zh"),
});
export const useLabTranslation = () => useContext(LabLocaleContext);

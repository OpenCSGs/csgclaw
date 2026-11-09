export type ClassValue = string | false | null | undefined;

export function classNames(...values: ClassValue[]): string {
  return values.filter((value): value is string => Boolean(value)).join(" ");
}

export function legacyClassName(name: string): string | undefined {
  return import.meta.env.CSGCLAW_UI_LEGACY_CLASSES ? name : undefined;
}

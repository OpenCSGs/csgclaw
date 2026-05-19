export type ClassValue = string | false | null | undefined;

export function classNames(...values: ClassValue[]): string {
  return values.filter((value): value is string => Boolean(value)).join(" ");
}

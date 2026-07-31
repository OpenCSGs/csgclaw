import { normalizeReasoningEffort } from "@/models/reasoning";
import type { TranslateFn } from "@/models/conversations";

export const reasoningOptionMessageKeys = {
  auto: "profileReasoningAuto",
  none: "profileReasoningDisabled",
  minimal: "profileReasoningMinimal",
  low: "profileReasoningLow",
  medium: "profileReasoningMedium",
  high: "profileReasoningHigh",
  xhigh: "profileReasoningXHigh",
} as const;

export function reasoningEffortLabel(t: TranslateFn, value: unknown): string {
  const normalized = normalizeReasoningEffort(value);
  const key = reasoningOptionMessageKeys[normalized as keyof typeof reasoningOptionMessageKeys];
  return t(key ?? "profileReasoningAuto");
}

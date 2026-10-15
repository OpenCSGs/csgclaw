import { DEFAULT_REASONING_EFFORT, REASONING_DISABLED_EFFORT, REASONING_SLIDER_STEPS } from "@/shared/constants/agents";

export function normalizeReasoningEffort(value: unknown): string {
  const normalized = String(value ?? "")
    .trim()
    .toLowerCase();
  if (normalized === "off") {
    return REASONING_DISABLED_EFFORT;
  }
  return normalized || DEFAULT_REASONING_EFFORT;
}

export function reasoningSliderIndex(value: unknown): number | null {
  const normalized = normalizeReasoningEffort(value);
  const index = REASONING_SLIDER_STEPS.findIndex((step) => step === normalized);
  return index < 0 ? null : index;
}

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

export function reasoningSliderIndex(value: unknown, steps: readonly string[] = REASONING_SLIDER_STEPS): number | null {
  const normalized = normalizeReasoningEffort(value);
  const index = steps.findIndex((step) => step === normalized);
  return index < 0 ? null : index;
}

export const supportedReasoningValues = ["none", "minimal", "low", "medium", "high", "xhigh", "max"] as const;
export type SupportedReasoningEffort = (typeof supportedReasoningValues)[number];

export function reasoningSteps(supported?: readonly string[]): readonly SupportedReasoningEffort[] {
  const values = supported?.filter((value): value is SupportedReasoningEffort =>
    supportedReasoningValues.includes(value as SupportedReasoningEffort),
  );
  return values ?? REASONING_SLIDER_STEPS;
}

export function normalizeReasoningEfforts(value: unknown): Record<string, string[]> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const entries = Object.entries(value).flatMap(([model, levels]) => {
    if (!Array.isArray(levels)) return [];
    const valid = levels.filter((level): level is SupportedReasoningEffort => supportedReasoningValues.includes(level));
    return [[model, valid] as const];
  });
  return entries.length ? Object.fromEntries(entries) : undefined;
}

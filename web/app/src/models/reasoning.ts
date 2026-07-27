import { DEFAULT_REASONING_EFFORT, REASONING_DISABLED_EFFORT } from "@/shared/constants/agents";

export function normalizeReasoningEffort(value: unknown): string {
  const normalized = String(value ?? "")
    .trim()
    .toLowerCase();
  if (normalized === "off") {
    return REASONING_DISABLED_EFFORT;
  }
  return normalized || DEFAULT_REASONING_EFFORT;
}

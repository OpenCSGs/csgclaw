import { errorMessage } from "@/api/client";
import type { TranslateFn } from "@/models/conversations";
import { localizeError } from "@/shared/i18n";

// Persisted failures contain the original diagnostic message. Keep that detail
// available separately, while presenting the same actionable copy in each locale.
export function memoryErrorMessage(error: unknown, t: TranslateFn): string {
  const code = error && typeof error === "object" && "code" in error ? String(error.code || "") : "";
  const detail = typeof error === "string" ? error : errorMessage(error, "");
  const raw = `${code} ${detail}`.trim();
  if (/timed?\s*out|timeout|deadline exceeded/i.test(raw)) return t("agentMemoryErrorTimeout");
  if (/memory_conflict|memory revision changed/i.test(raw)) return t("agentMemoryErrorConflict");
  if (/\bmemory_invalid\b/i.test(raw)) return t("agentMemoryErrorContent");
  if (/memory_capacity|memory capacity exceeded/i.test(raw)) return t("agentMemoryErrorCapacity");
  if (/memory task expired/i.test(raw)) return t("agentMemoryErrorExpired");
  if (/memory_unavailable/i.test(raw)) return t("agentMemoryUnavailable");
  if (
    /memory_storage_error|no space left|read-only file system|\b(?:open|mkdir|rename|write|read)\s+\S+.*permission denied/i.test(
      raw,
    )
  )
    return t("agentMemoryErrorStorage");
  if (
    /invalid memory update|memory model (?:returned|response (?:is|contains)).*(invalid|incomplete|missing|unexpected)|unknown (message|entry) reference/i.test(
      raw,
    )
  )
    return t("agentMemoryErrorResult");
  // Reuse shared provider translations (model missing, auth, quota, permissions,
  // unsupported features, context limits, etc.) instead of duplicating that copy.
  const localized = localizeError(raw, t);
  if (localized && localized !== raw) return localized;
  const status =
    error && typeof error === "object" && "status" in error && typeof error.status === "number"
      ? String(error.status)
      : /\bHTTP\s+(\d{3})\b/i.exec(raw)?.[1];
  const statusKeys: Record<string, string> = {
    "401": "invalid_api_key",
    "403": "forbidden",
    "408": "upstream_timeout",
    "413": "context_length_exceeded",
    "429": "rate_limit_exceeded",
  };
  if (status === "404") return t("agentMemoryErrorNotFound");
  if (status && statusKeys[status]) return t(`errors.${statusKeys[status]}`);
  if (status && Number(status) >= 500) return t("errors.upstream_response_invalid");
  if (/cannot reach|failed to fetch|network|connection|dial tcp|no such host|TLS handshake/i.test(raw))
    return t("errors.upstream_unavailable");
  return t("agentMemoryErrorUnknown");
}

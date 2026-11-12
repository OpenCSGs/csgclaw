import { describe, expect, it } from "vitest";
import { createTranslator } from "@/shared/i18n";
import { memoryErrorMessage } from "./memoryError";

const failures = [
  [
    "Memory model request failed (HTTP 404, model_not_found): The selected model does not exist.",
    "errors.model_not_found",
  ],
  ["Memory extraction timed out. Please retry.", "agentMemoryErrorTimeout"],
  ["context deadline exceeded", "agentMemoryErrorTimeout"],
  ["HTTP 401: invalid_api_key", "errors.invalid_api_key"],
  ["Memory model request failed (HTTP 401, invalid_api_key): authentication failed.", "errors.invalid_api_key"],
  [
    "Memory model request failed (HTTP 400, invalid_request_error): check model configuration.",
    "errors.invalid_request_error",
  ],
  ["HTTP 403: permission denied", "errors.forbidden"],
  ["HTTP 403: OpenAI permission denied", "errors.forbidden"],
  ["HTTP 429: rate limit exceeded", "errors.rate_limit_exceeded"],
  ["HTTP 429, insufficient_balance", "errors.insufficient_balance"],
  ["HTTP 404: unknown route", "agentMemoryErrorNotFound"],
  ["HTTP 503: temporarily unavailable", "errors.upstream_response_invalid"],
  ["Cannot reach the model service", "errors.upstream_unavailable"],
  ["invalid memory update: model cited a source_message_id outside the supplied messages", "agentMemoryErrorResult"],
  ["Memory model returned an invalid change set: unexpected end of JSON input", "agentMemoryErrorResult"],
  ["Memory model returned an incomplete response", "agentMemoryErrorResult"],
  ["memory capacity exceeded", "agentMemoryErrorCapacity"],
  ["Memory task expired before it could complete. Please retry.", "agentMemoryErrorExpired"],
  ["memory revision changed", "agentMemoryErrorConflict"],
  ["open /fixture/memory/state.json: permission denied", "agentMemoryErrorStorage"],
  ["HTTP 400, context_length_exceeded", "errors.context_length_exceeded"],
  ["HTTP 400, unsupported_reasoning_effort", "errors.unsupported_reasoning_effort"],
  ["New unclassified provider failure containing internal details", "agentMemoryErrorUnknown"],
] as const;

for (const locale of ["zh", "en"] as const) {
  describe(`memory errors in ${locale}`, () => {
    const t = createTranslator(locale);
    it.each(failures)("explains %s", (raw, key) => {
      const message = memoryErrorMessage(raw, t);
      expect(message).toBe(t(key));
      expect(message).not.toBe(key);
      expect(message).not.toBe(raw);
    });
    it("uses API codes for memory mutations without exposing diagnostics", () => {
      expect(memoryErrorMessage({ code: "memory_invalid", message: "Invalid memory update" }, t)).toBe(
        t("agentMemoryErrorContent"),
      );
      expect(memoryErrorMessage({ status: 401, message: "Authentication failed." }, t)).toBe(
        t("errors.invalid_api_key"),
      );
      expect(memoryErrorMessage({ code: "memory_conflict", message: "Changed revision 12" }, t)).toBe(
        t("agentMemoryErrorConflict"),
      );
    });
  });
}

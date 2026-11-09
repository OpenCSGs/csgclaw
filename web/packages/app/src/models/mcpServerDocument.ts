import { mcpServerPayloadFromDocument, type MCPServerPayload } from "./mcp";
import type { TranslateFn } from "./conversations";
export type MCPServerDocumentParseResult =
  | {
      kind: "valid";
      payload: MCPServerPayload;
    }
  | {
      kind: "structure" | "syntax";
      message: string;
    };
export function parseMCPServerDocument(value: string, t: TranslateFn): MCPServerDocumentParseResult {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    return { kind: "syntax", message: t("resourcesMCPServerDocumentInvalid") };
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    return { kind: "structure", message: t("resourcesMCPServerDocumentObjectRequired") };
  }
  const payload = mcpServerPayloadFromDocument(parsed);
  if (!payload) {
    return { kind: "structure", message: t("resourcesMCPServerDocumentInvalidShape") };
  }
  return { kind: "valid", payload };
}

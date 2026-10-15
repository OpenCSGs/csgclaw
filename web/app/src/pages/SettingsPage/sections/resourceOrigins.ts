import { mcpServerOrigin, type MCPServer } from "@/models/mcp";
import { skillOriginKind, type SkillSummary } from "@/models/skillhub";
import type { TranslateFn } from "@/models/conversations";
export function mcpOriginLabel(server: MCPServer, t: TranslateFn): string {
  const origin = mcpServerOrigin(server);
  if (origin === "builtin") {
    return t("iaMCPBadgeBuiltin");
  }
  if (origin === "knowledge") {
    return t("iaSettingsCardKnowledge");
  }
  return origin === "market" ? t("iaMCPBadgeMarket") : t("iaMCPBadgeManual");
}
export function skillOriginLabel(skill: SkillSummary, t: TranslateFn): string {
  const kind = skillOriginKind(skill);
  if (kind === "builtin") {
    return t("resourcesSkillSystemBadge");
  }
  if (kind === "unknown") {
    return t("iaSkillsBadgeUnknown");
  }
  return kind === "market" ? t("iaSkillsBadgeMarket") : t("iaSkillsBadgeLocal");
}

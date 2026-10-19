import type { HubTemplate } from "@/models/hubWorkspace";

export type TemplateGroup = "all" | "official" | "mine" | "public";

export function templateNamespace(template: HubTemplate): string {
  return String(
    template.namespace ||
      String(template.id || "")
        .split("/")
        .slice(0, -1)
        .join("/"),
  )
    .trim()
    .toLowerCase();
}

export function isOfficialTemplate(template: HubTemplate): boolean {
  return (
    template.source?.kind === "builtin" ||
    (template.source?.kind === "remote" && templateNamespace(template) === "agentichub")
  );
}

export function templateInGroup(template: HubTemplate, group: TemplateGroup, username = ""): boolean {
  const official = isOfficialTemplate(template);
  const own = Boolean(username.trim()) && templateNamespace(template) === username.trim().toLowerCase();
  if (group === "official") return official;
  if (group === "mine")
    return template.source?.kind === "local" || (!official && template.source?.kind === "remote" && own);
  if (group === "public") return template.source?.kind === "remote" && !official && !own;
  return true;
}

export function templateSourceLabelKey(template: HubTemplate, username = ""): string {
  if (isOfficialTemplate(template)) return "taskEntryOverview_official";
  if (template.source?.kind === "local") return "resourcesSkillLocalFilter";
  if (template.source?.kind === "remote") {
    return username.trim() && templateNamespace(template) === username.trim().toLowerCase()
      ? "iaTemplateCloudTag"
      : "resourcesPublicTemplates";
  }
  return "resourcesSkillAllFilter";
}

export function hasSameNamedCloudTemplate(
  template: HubTemplate,
  templates: readonly HubTemplate[],
  username = "",
): boolean {
  if (template.source?.kind !== "local" || !template.name?.trim() || !username.trim()) return false;
  return templates.some(
    (remote) =>
      remote.source?.kind === "remote" &&
      templateNamespace(remote) === username.trim().toLowerCase() &&
      remote.name === template.name,
  );
}

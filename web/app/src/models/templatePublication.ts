import type { HubTemplate } from "@/models/hubWorkspace";

function siteOrigin(site: string): string {
  try {
    return new URL(site).origin;
  } catch {
    return "";
  }
}

export function templateVersionLabel(template: HubTemplate | null | undefined): string {
  return template?.revision && template.revision > 0 ? `V${template.revision}` : template?.version?.trim() || "";
}

export function templatePublicationTarget(
  template: HubTemplate | null | undefined,
  templates: readonly HubTemplate[],
  username: string,
  site: string,
) {
  if (template?.source?.kind !== "local" || !username.trim()) return null;
  const namespace = username.trim().toLowerCase();
  const origin = siteOrigin(site);
  const publication = template.publications?.find(
    (item) => origin && siteOrigin(item.site) === origin && item.namespace.toLowerCase() === namespace,
  );
  const remote = templates.find(
    (item) =>
      item.source?.kind === "remote" &&
      (publication
        ? item.id === publication.template_id
        : item.name === template.name && (item.namespace || item.id?.split("/")[0] || "").toLowerCase() === namespace),
  );
  const id = publication?.template_id || remote?.id;
  return id ? { id, publication } : null;
}

// Only compare revisions when both templates carry the same persistent identity.
export function templatePublicationStatus(local: HubTemplate, remote?: HubTemplate | null) {
  if (!remote || !local.uid || remote.uid !== local.uid || !local.revision || !remote.revision) return "unknown";
  return local.revision === remote.revision ? "current" : "different";
}

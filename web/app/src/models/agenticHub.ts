export function agenticHubCreateAgentURL(siteURL: string, templateID: string): string {
  if (!templateID.trim()) return "";
  try {
    const site = new URL(siteURL);
    if (site.protocol !== "https:" && site.protocol !== "http:") return "";
    const destination = new URL("/agentichub", site.origin);
    destination.searchParams.set("action", "create-agent");
    destination.searchParams.set("template_name", templateID);
    return destination.toString();
  } catch {
    return "";
  }
}

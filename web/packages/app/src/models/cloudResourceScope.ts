// A cache/draft scope, not a local runtime agent ID. Endpoint credentials stay server-side.
const prefix = "cloud-resource:";
export function cloudResourceScope(agentID: string, connectionID: string): string {
  return prefix + encodeURIComponent(JSON.stringify([agentID, connectionID]));
}
export function parseCloudResourceScope(scope: string): { agentID: string; connectionID: string } | null {
  if (!scope.startsWith(prefix)) return null;
  try {
    const value: unknown = JSON.parse(decodeURIComponent(scope.slice(prefix.length)));
    if (
      Array.isArray(value) &&
      value.length === 2 &&
      value.every((item) => typeof item === "string" && item.length > 0)
    ) {
      return { agentID: value[0], connectionID: value[1] };
    }
  } catch {
    /* Invalid scopes cannot select a remote endpoint. */
  }
  return null;
}

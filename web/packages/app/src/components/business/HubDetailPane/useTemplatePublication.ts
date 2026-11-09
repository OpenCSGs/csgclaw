import { useEffect, useState } from "react";
import { fetchHubTemplate } from "@/api/hub";
import type { HubTemplate } from "@/models/hubWorkspace";
import { templatePublicationTarget } from "@/models/templatePublication";

export function useTemplatePublication(
  template: HubTemplate | null | undefined,
  templates: readonly HubTemplate[],
  username: string,
  site: string,
  refresh: unknown,
) {
  const target = templatePublicationTarget(template, templates, username, site);
  const id = target?.id || "";
  const [result, setResult] = useState<{ key: string; remote: HubTemplate } | null>(null);
  const key = JSON.stringify([
    id,
    site,
    username,
    template?.id,
    template?.revision,
    target?.publication?.published_at,
    refresh,
  ]);
  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    fetchHubTemplate(id)
      .then((remote) => {
        if (!cancelled && remote.id === id) setResult({ key, remote });
      })
      .catch(() => {
        // A local publication record remains useful when the community is unavailable.
      });
    return () => {
      cancelled = true;
    };
  }, [id, key]);
  return { target, remote: result?.key === key ? result.remote : null };
}

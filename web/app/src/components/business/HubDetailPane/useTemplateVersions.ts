import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import {
  fetchTemplateVersions,
  fetchTemplateVersionWorkspace,
  fetchTemplateVersionFile,
  restoreTemplateVersion,
  type TemplateVersion,
} from "@/api/hub";
import type { HubTemplate, HubWorkspaceFile, HubWorkspaceListing } from "@/models/hubWorkspace";
import type { TranslateFn } from "@/models/conversations";
import { localizeAPIError } from "@/shared/i18n";

export function useTemplateVersions(
  template: HubTemplate,
  t: TranslateFn,
  onRestored?: (template: HubTemplate) => unknown,
) {
  const queryClient = useQueryClient();
  const [versions, setVersions] = useState<TemplateVersion[]>([]);
  const [revision, setRevision] = useState<number | null>(null);
  const [listing, setListing] = useState<HubWorkspaceListing | null>(null);
  const [file, setFile] = useState<HubWorkspaceFile | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const generation = useRef(0);
  const id = String(template.id || "");
  useEffect(() => {
    const request = ++generation.current;
    setBusy(true);
    setError("");
    setRevision(null);
    setListing(null);
    setFile(null);
    fetchTemplateVersions(id)
      .then((items) => {
        if (generation.current === request) setVersions([...items].reverse());
      })
      .catch((err: unknown) => {
        if (generation.current === request) setError(localizeAPIError(err, t, t("agentActionFailed")));
      })
      .finally(() => {
        if (generation.current === request) setBusy(false);
      });
    return () => {
      generation.current = request + 1;
    };
  }, [id, template.revision, t]);

  async function browse(selectedRevision: number, path = "", isFile = false) {
    const request = ++generation.current;
    setBusy(true);
    setError("");
    if (selectedRevision !== revision) {
      setListing(null);
      setFile(null);
    } else if (!isFile) {
      setFile(null);
    }
    setRevision(selectedRevision);
    try {
      if (isFile) {
        const result = await fetchTemplateVersionFile(id, selectedRevision, path);
        if (generation.current === request) setFile(result);
      } else {
        const result = await fetchTemplateVersionWorkspace(id, selectedRevision, path);
        if (generation.current === request) {
          setListing(result);
          setFile(null);
        }
      }
    } catch (err) {
      if (generation.current === request) setError(localizeAPIError(err, t, t("agentActionFailed")));
      return false;
    } finally {
      if (generation.current === request) setBusy(false);
    }
  }

  async function rollback(targetRevision: number) {
    if (busy) return false;
    const request = ++generation.current;
    setBusy(true);
    setError("");
    try {
      const result = await restoreTemplateVersion(id, targetRevision);
      if (generation.current !== request) return;
      setRevision(null);
      setListing(null);
      setFile(null);
      queryClient.setQueryData<HubTemplate[]>(workspaceQueryKeys.hubTemplates(), (current) =>
        current?.map((item) => (item.id === result.id ? result : item)),
      );
      queryClient.setQueryData(workspaceQueryKeys.hubTemplate(result.id), result);
      void queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.hubWorkspaceScope(result.id) });
      void queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.hubWorkspaceFileScope(result.id) });
      onRestored?.(result);
      return true;
    } catch (err) {
      if (generation.current === request) setError(localizeAPIError(err, t, t("agentActionFailed")));
      return false;
    } finally {
      if (generation.current === request) setBusy(false);
    }
  }
  return { versions, revision, listing, file, busy, error, browse, rollback };
}

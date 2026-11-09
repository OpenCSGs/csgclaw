import { useEffect, useEffectEvent, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { workspaceQueryKeys } from "@/hooks/workspace/workspaceQueries";
import {
  fetchTemplateVersions,
  deleteTemplateVersion,
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
  const activeRevision = template.revision || 1;
  const reportLoadError = useEffectEvent((err: unknown) => {
    setError(localizeAPIError(err, t, t("agentActionFailed")));
  });
  useEffect(() => {
    const request = ++generation.current;
    let cancelled = false;
    setBusy(true);
    setError("");
    setRevision(null);
    setListing(null);
    setFile(null);
    fetchTemplateVersions(id)
      .then(async (items) => {
        if (cancelled || generation.current !== request) return;
        setVersions([...items].reverse());
        if (!items.some((item) => item.revision === activeRevision)) return;
        setRevision(activeRevision);
        const result = await fetchTemplateVersionWorkspace(id, activeRevision, "");
        if (!cancelled && generation.current === request) setListing(result);
      })
      .catch((err: unknown) => {
        if (!cancelled && generation.current === request) reportLoadError(err);
      })
      .finally(() => {
        if (!cancelled && generation.current === request) setBusy(false);
      });
    return () => {
      cancelled = true;
    };
  }, [id, activeRevision]);

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
  async function remove(selectedRevision: number) {
    if (busy) return false;
    const request = ++generation.current;
    setBusy(true);
    setError("");
    try {
      await deleteTemplateVersion(id, selectedRevision);
      if (generation.current !== request) return;
      const remaining = versions.filter((item) => item.revision !== selectedRevision);
      setVersions(remaining);
      const result = { ...template, version_count: remaining.length };
      queryClient.setQueryData<HubTemplate[]>(workspaceQueryKeys.hubTemplates(), (current) =>
        current?.map((item) => (item.id === id ? { ...item, version_count: remaining.length } : item)),
      );
      queryClient.setQueryData(workspaceQueryKeys.hubTemplate(id), result);
      onRestored?.(result);
      if (revision === selectedRevision) {
        setRevision(null);
        setListing(null);
        setFile(null);
      }
      return true;
    } catch (err) {
      if (generation.current === request) setError(localizeAPIError(err, t, t("agentActionFailed")));
      return false;
    } finally {
      if (generation.current === request) setBusy(false);
    }
  }
  function collapse() {
    ++generation.current;
    setRevision(null);
    setListing(null);
    setFile(null);
    setBusy(false);
  }
  return { versions, revision, listing, file, busy, error, browse, rollback, remove, collapse };
}

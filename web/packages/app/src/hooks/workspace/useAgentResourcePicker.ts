import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchSkills, installRemoteSkillRequest, uploadSkillArchive } from "@/api/skills";
import { createMCPServerRequest, installRemoteMCPServerRequest } from "@/api/mcp";
import { fetchRemoteKnowledgeBaseMCPConfig } from "@/api/knowledgeBases";
import { fetchAgentSkillSummaries, fetchAgentMCPServers, type AgentSkillInstallOptions } from "@/api/agents";
import { errorMessage } from "@/api/client";
import type { TranslateFn } from "@/models/conversations";
import { remoteSkillInstallName, type SkillSummary } from "@/models/skillhub";
import {
  knowledgeBaseMCPServer,
  mergeRemoteKnowledgeBasePages,
  type RemoteKnowledgeBase,
} from "@/models/knowledgeBases";
import {
  createdMCPServerName,
  isRemoteMCPInstalled,
  mcpServersFromCatalogResponse,
  mcpServersFromMap,
  type MCPServerPayload,
  type RemoteMCPServer,
} from "@/models/mcp";
import { localizeAPIError } from "@/shared/i18n";
import {
  workspaceQueryKeys,
  useWorkspaceMCPServersQuery,
  useWorkspaceOfficialSkillsQuery,
  useWorkspaceRemoteMCPServersQuery,
  useWorkspaceKnowledgeBasesQuery,
} from "./workspaceQueries";

export type AgentResourcePickerKind = "skill" | "mcp" | "knowledge";

function marketSkillKey(skill: SkillSummary): string {
  return JSON.stringify(["skill", skill.remotePath?.trim() || "", skill.remoteRef?.trim() || ""]);
}

export function useAgentResourcePicker({
  agentID,
  kind,
  authenticated,
  onAdd,
  t,
}: {
  agentID: string;
  kind: AgentResourcePickerKind;
  authenticated: boolean;
  onAdd: (names: string[], options?: AgentSkillInstallOptions) => Promise<boolean> | boolean;
  t: TranslateFn;
}) {
  const client = useQueryClient();
  const [market, setMarket] = useState(false);
  const [search, setSearch] = useState("");
  const [querySearch, setQuerySearch] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const active = useRef(true);
  const pending = useRef(false);
  const prepared = useRef(new Map<string, string>());
  const marketSkillNames = useRef(new Map<string, string>());
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  useEffect(() => {
    const timer = window.setTimeout(() => setQuerySearch(search.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [search]);
  const skills = useQuery({ queryKey: workspaceQueryKeys.skills(), queryFn: fetchSkills, enabled: kind === "skill" });
  const agentSkills = useQuery({
    queryKey: workspaceQueryKeys.agentSkills(agentID),
    queryFn: ({ signal }) => fetchAgentSkillSummaries(agentID, signal),
    enabled: kind === "skill",
  });
  const servers = useWorkspaceMCPServersQuery({ enabled: kind !== "skill" });
  const agentServers = useQuery({
    queryKey: workspaceQueryKeys.agentMCPServers(agentID),
    queryFn: () => fetchAgentMCPServers(agentID),
    enabled: kind !== "skill",
  });
  const skillMarket = useWorkspaceOfficialSkillsQuery(querySearch, {
    enabled: kind === "skill" && market && authenticated,
  });
  const mcpMarket = useWorkspaceRemoteMCPServersQuery(querySearch, {
    enabled: kind === "mcp" && market && authenticated,
  });
  const knowledge = useWorkspaceKnowledgeBasesQuery(querySearch, { enabled: kind === "knowledge" && authenticated });
  const globalServers = mcpServersFromCatalogResponse(servers.data ?? null);
  const installedNames = new Set(
    kind === "skill"
      ? (agentSkills.data ?? []).map((skill) => skill.name)
      : Object.keys(agentServers.data?.servers ?? {}),
  );
  const enabledNames = new Set(
    kind === "skill"
      ? (agentSkills.data ?? []).filter((skill) => skill.enabled !== false).map((skill) => skill.name)
      : mcpServersFromMap(agentServers.data?.servers)
          .filter((server) => server.config.enabled !== false)
          .map((server) => server.name),
  );

  function skillAction(skill: SkillSummary): "install" | "replace" {
    const key = marketSkillKey(skill);
    const preparedName = prepared.current.get(key);
    const name = preparedName || marketSkillNames.current.get(key) || remoteSkillInstallName(skill);
    const installed = preparedName || installedNames.has(name) || skills.data?.some((item) => item.name === name);
    return installed ? "replace" : "install";
  }

  async function run(key: string, acquire: () => Promise<string[]>, options?: AgentSkillInstallOptions) {
    if (pending.current) return false;
    pending.current = true;
    setBusy(key);
    setError("");
    try {
      const names = await acquire();
      if (!active.current) return false;
      return await (options ? onAdd(names, options) : onAdd(names));
    } catch (failure) {
      if (active.current) setError(localizeAPIError(failure, t, errorMessage(failure)));
      return false;
    } finally {
      pending.current = false;
      if (active.current) setBusy("");
    }
  }

  async function acquireSkill(key: string, acquire: () => Promise<SkillSummary>) {
    const existing = prepared.current.get(key);
    if (existing) return [existing];
    const skill = await acquire();
    for (const [preparedKey, name] of prepared.current) {
      if (name === skill.name) prepared.current.delete(preparedKey);
    }
    prepared.current.set(key, skill.name);
    client.setQueryData<SkillSummary[]>(workspaceQueryKeys.skills(), (current) => [
      ...(current ?? []).filter((item) => item.name !== skill.name),
      skill,
    ]);
    await Promise.all([
      client.invalidateQueries({ queryKey: workspaceQueryKeys.skills() }),
      client.invalidateQueries({ queryKey: workspaceQueryKeys.skillTreeScope() }),
      client.invalidateQueries({ queryKey: workspaceQueryKeys.skillFileScope() }),
    ]);
    return [skill.name];
  }

  async function acquireMCP(key: string, acquire: () => Promise<string>) {
    const existing = prepared.current.get(key);
    if (existing) return [existing];
    const name = await acquire();
    prepared.current.set(key, name);
    await Promise.all([
      client.invalidateQueries({ queryKey: workspaceQueryKeys.mcpServers() }),
      client.invalidateQueries({ queryKey: workspaceQueryKeys.knowledgeBasesScope() }),
    ]);
    return [name];
  }

  async function createCatalogMCPServer(payload: MCPServerPayload): Promise<string> {
    const state = await createMCPServerRequest(payload);
    client.setQueryData(workspaceQueryKeys.mcpServers(), state);
    const name = createdMCPServerName(state, payload);
    if (!name) throw new Error(t("profileMCPIdentityMissing"));
    return name;
  }

  return {
    market,
    setMarket,
    search,
    setSearch,
    busy,
    error,
    installedNames,
    enabledNames,
    skillAction,
    skills: skills.data ?? [],
    globalServers,
    libraryReady:
      kind === "skill"
        ? skills.data !== undefined && agentSkills.data !== undefined
        : servers.data !== undefined && agentServers.data !== undefined,
    libraryLoading:
      kind === "skill" ? skills.isFetching || agentSkills.isFetching : servers.isFetching || agentServers.isFetching,
    libraryError: kind === "skill" ? skills.error || agentSkills.error : servers.error || agentServers.error,
    retryLibrary: () =>
      kind === "skill"
        ? Promise.all([skills.refetch(), agentSkills.refetch()])
        : Promise.all([servers.refetch(), agentServers.refetch()]),
    skillMarket,
    mcpMarket,
    knowledge,
    knowledgeItems: mergeRemoteKnowledgeBasePages(knowledge.data?.pages ?? []),
    add: (names: string[]) => run("library", async () => names),
    installSkill: async (skill: SkillSummary) => {
      const key = marketSkillKey(skill);
      const replace = skillAction(skill) === "replace";
      const added = await run(
        skill.remotePath || skill.name,
        () =>
          acquireSkill(key, async () => {
            const installed = await installRemoteSkillRequest(skill.remotePath || "", skill.remoteRef, replace);
            marketSkillNames.current.set(key, installed.name);
            return installed;
          }),
        { replace: true },
      );
      if (added) prepared.current.delete(key);
      return added;
    },
    upload: (file: File) =>
      run("upload", () =>
        acquireSkill(`upload:${file.name}:${file.size}:${file.lastModified}`, () => uploadSkillArchive(file)),
      ),
    installMCP: (server: RemoteMCPServer) =>
      run(server.id, () => {
        const existing = globalServers.find((item) => isRemoteMCPInstalled([item], server));
        return existing
          ? Promise.resolve([existing.name])
          : acquireMCP(server.id, () => installRemoteMCPServerRequest(server.id));
      }),
    createMCP: (payload: MCPServerPayload) =>
      run("manual", () => acquireMCP(JSON.stringify(payload), () => createCatalogMCPServer(payload))),
    connectKnowledge: (item: RemoteKnowledgeBase) =>
      run(item.id, () => {
        const existing = knowledgeBaseMCPServer(item, globalServers);
        return existing
          ? Promise.resolve([existing.name])
          : acquireMCP(`knowledge:${item.id}`, async () => {
              const payload = await fetchRemoteKnowledgeBaseMCPConfig(item.id);
              return createCatalogMCPServer(payload);
            });
      }),
  };
}

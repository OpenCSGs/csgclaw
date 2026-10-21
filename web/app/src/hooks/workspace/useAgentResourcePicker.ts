import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { fetchSkills, installRemoteSkillRequest, uploadSkillArchive } from "@/api/skills";
import { createMCPServerRequest, installRemoteMCPServerRequest } from "@/api/mcp";
import { fetchRemoteKnowledgeBaseMCPConfig } from "@/api/knowledgeBases";
import { fetchAgentSkillSummaries, fetchAgentMCPServers } from "@/api/agents";
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
  onAdd: (names: string[]) => Promise<boolean> | boolean;
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

  async function run(key: string, acquire: () => Promise<string[]>) {
    if (pending.current) return false;
    pending.current = true;
    setBusy(key);
    setError("");
    try {
      const names = await acquire();
      if (!active.current) return false;
      return await onAdd(names);
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
    prepared.current.set(key, skill.name);
    client.setQueryData<SkillSummary[]>(workspaceQueryKeys.skills(), (current) => [
      ...(current ?? []).filter((item) => item.name !== skill.name),
      skill,
    ]);
    await client.invalidateQueries({ queryKey: workspaceQueryKeys.skills() });
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
    isSkillEnabled: (skill: SkillSummary) => {
      const name = prepared.current.get(marketSkillKey(skill));
      return Boolean(name && installedNames.has(name));
    },
    skills: skills.data ?? [],
    globalServers,
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
    installSkill: (skill: SkillSummary) =>
      run(skill.remotePath || skill.name, () =>
        acquireSkill(marketSkillKey(skill), async () => {
          const name = remoteSkillInstallName(skill);
          if (skills.data?.some((item) => item.name === name)) {
            throw new Error(t("profileSkillNameConflict", { name }));
          }
          return installRemoteSkillRequest(skill.remotePath || "", skill.remoteRef);
        }),
      ),
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

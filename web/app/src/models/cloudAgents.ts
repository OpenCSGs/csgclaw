export type CloudAgent = {
  id: string;
  relayURL: string;
  connectionID: string;
  name: string;
  description: string;
  owner: string;
  creatorName?: string;
  modelID?: string;
  templateRepoPath?: string;
  publisher: string;
  sourceAgentID: string;
  visibility: "public" | "private";
  status: "online" | "offline";
  lastSeenAt: string | null;
  avatar: string;
};

// Persist metadata and a server-side credential reference, never a token.
export function parseCloudAgents(raw: string | null): CloudAgent[] {
  try {
    const value: unknown = JSON.parse(raw || "[]");
    if (!Array.isArray(value)) return [];
    const agents = new Map<string, CloudAgent>();
    for (const candidate of value) {
      if (!candidate || typeof candidate !== "object") continue;
      const entry = candidate as Record<string, unknown>;
      if (
        typeof entry.id !== "string" ||
        !entry.id ||
        typeof entry.connectionID !== "string" ||
        !entry.connectionID ||
        typeof entry.relayURL !== "string"
      )
        continue;
      try {
        const url = new URL(entry.relayURL);
        if (!["http:", "https:"].includes(url.protocol) || url.username || url.password || url.search || url.hash)
          continue;
      } catch {
        continue;
      }
      if (
        !["name", "description", "owner", "publisher", "sourceAgentID", "avatar"].every(
          (key) => typeof entry[key] === "string",
        )
      )
        continue;
      if (entry.visibility !== "public" && entry.visibility !== "private") continue;
      // Explicitly copy permitted fields to avoid preserving unexpected credentials.
      agents.set(JSON.stringify([entry.relayURL, entry.connectionID, entry.id]), {
        id: entry.id,
        relayURL: entry.relayURL,
        connectionID: entry.connectionID,
        name: String(entry.name),
        description: String(entry.description),
        owner: String(entry.owner),
        creatorName: typeof entry.creatorName === "string" ? entry.creatorName.trim() : undefined,
        modelID: typeof entry.modelID === "string" ? entry.modelID.trim() : undefined,
        publisher: String(entry.publisher),
        sourceAgentID: String(entry.sourceAgentID),
        avatar: String(entry.avatar),
        visibility: entry.visibility,
        status: "offline",
        lastSeenAt:
          typeof entry.lastSeenAt === "string" && Number.isFinite(Date.parse(entry.lastSeenAt))
            ? entry.lastSeenAt
            : null,
      });
    }
    return [...agents.values()];
  } catch {
    return [];
  }
}

export function sameCloudAgentReference(a: CloudAgent, b: CloudAgent): boolean {
  return a.id === b.id && a.relayURL === b.relayURL && a.connectionID === b.connectionID;
}

export function filterCloudAgents(agents: readonly CloudAgent[], query: string): CloudAgent[] {
  const search = query.trim().toLocaleLowerCase();
  return agents.filter((agent) =>
    [agent.name, agent.description, agent.owner, agent.id].join(" ").toLocaleLowerCase().includes(search),
  );
}

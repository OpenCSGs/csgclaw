import type { JSONRecord } from "@/models/agents";

export type MCPServer = {
  config: JSONRecord;
  description?: string;
  name: string;
};

export type MCPServerPayload = {
  config: JSONRecord;
  name: string;
};

export function mcpServersFromCatalogResponse(response: unknown): MCPServer[] {
  return mcpServersFromMap(mcpServerMapFromCatalogResponse(response));
}

export function mcpServersFromMap(servers: unknown): MCPServer[] {
  if (!isJSONRecord(servers)) {
    return [];
  }
  return Object.entries(servers)
    .reduce<MCPServer[]>((items, [name, value]) => {
      const normalizedName = String(name || "").trim();
      if (!normalizedName || !isJSONRecord(value)) {
        return items;
      }
      const config = cloneJSONRecord(value);
      items.push({
        name: normalizedName,
        config,
        description: mcpServerDescription(config),
      });
      return items;
    }, [])
    .sort((left, right) => left.name.localeCompare(right.name));
}

export function mcpServersMap(servers: unknown): Record<string, JSONRecord> {
  return cloneMCPServersRecord(isJSONRecord(servers) ? servers : null);
}

export function mcpServerPayloadFromDocument(document: unknown): MCPServerPayload | null {
  const entries = Object.entries(cloneMCPServersRecord(mcpServerMapFromCatalogResponse(document)));
  if (entries.length !== 1) {
    return null;
  }
  const [name, serverConfig] = entries[0];
  return {
    name,
    config: serverConfig,
  };
}

export function formatMCPServerDocument(name: string, config: JSONRecord): string {
  return JSON.stringify(
    {
      mcpServers: {
        [String(name || "").trim()]: cloneJSONRecord(config),
      },
    },
    null,
    2,
  );
}

export function mcpServerDescription(config: JSONRecord | null | undefined): string {
  if (!config) {
    return "";
  }
  const explicit = String(config.description ?? "").trim();
  if (explicit) {
    return explicit;
  }
  const command = String(config.command ?? "").trim();
  const args = Array.isArray(config.args) ? config.args.map((item) => String(item ?? "").trim()).filter(Boolean) : [];
  if (command) {
    return [command, ...args].join(" ");
  }
  const url = String(config.url ?? "").trim();
  if (url) {
    return url;
  }
  const transport = String(config.transport ?? config.type ?? "").trim();
  return transport;
}

export function cloneJSONRecord(value: JSONRecord): JSONRecord {
  try {
    return JSON.parse(JSON.stringify(value)) as JSONRecord;
  } catch {
    return { ...value };
  }
}

function mcpServerMapFromCatalogResponse(value: unknown): Record<string, unknown> | null {
  if (!isJSONRecord(value)) {
    return null;
  }
  return isJSONRecord(value.mcpServers) ? (value.mcpServers as Record<string, unknown>) : null;
}

function cloneMCPServersRecord(value: Record<string, unknown> | null | undefined): Record<string, JSONRecord> {
  const out: Record<string, JSONRecord> = {};
  Object.entries(value || {}).forEach(([name, config]) => {
    const normalizedName = String(name || "").trim();
    if (!normalizedName || !isJSONRecord(config)) {
      return;
    }
    out[normalizedName] = cloneJSONRecord(config);
  });
  return out;
}

function isJSONRecord(value: unknown): value is JSONRecord {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

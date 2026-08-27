import type { JSONRecord } from "@/models/agents";

export type MCPServer = {
  config: JSONRecord;
  description?: string;
  name: string;
};

export type RemoteMCPServer = {
  description?: string;
  id: string;
  name: string;
  protocol?: string;
  url?: string;
};

export type MCPServerPayload = {
  config: JSONRecord;
  name: string;
};

export type MCPProbeServerInfo = {
  name?: string;
  title?: string;
  version?: string;
};

export type MCPProbeTool = {
  description?: string;
  inputSchema?: JSONRecord;
  name: string;
  outputSchema?: unknown;
  title?: string;
};

export type MCPProbeResult = {
  connected: boolean;
  durationMs: number;
  protocolVersion?: string;
  serverInfo?: MCPProbeServerInfo;
  tools: MCPProbeTool[];
  toolsSupported: boolean;
  truncated: boolean;
};

export type MCPToolParameter = {
  description?: string;
  name: string;
  required: boolean;
  type: string;
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

export function mcpServersFromTemplateDocument(document: unknown): MCPServer[] {
  if (!isJSONRecord(document)) {
    return [];
  }
  const keys = Object.keys(document);
  const servers = keys.length === 1 && isJSONRecord(document.mcpServers) ? document.mcpServers : document;
  return mcpServersFromMap(servers);
}

export function mcpServersMap(servers: unknown): Record<string, JSONRecord> {
  return cloneMCPServersRecord(isJSONRecord(servers) ? servers : null);
}

export function hasMCPServerName(servers: readonly MCPServer[], name: string | null | undefined): boolean {
  const normalizedName = String(name || "").trim();
  return Boolean(normalizedName && servers.some((server) => server.name === normalizedName));
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

export function mcpProbeResultFromResponse(response: unknown): MCPProbeResult | null {
  if (!isJSONRecord(response) || response.connected !== true || !Array.isArray(response.tools)) {
    return null;
  }
  const tools = response.tools.reduce<MCPProbeTool[]>((items, rawTool) => {
    if (!isJSONRecord(rawTool)) {
      return items;
    }
    const name = stringFromUnknown(rawTool.name);
    if (!name) {
      return items;
    }
    items.push({
      description: stringFromUnknown(rawTool.description) || undefined,
      inputSchema: isJSONRecord(rawTool.input_schema) ? cloneJSONRecord(rawTool.input_schema) : undefined,
      name,
      outputSchema: rawTool.output_schema,
      title: stringFromUnknown(rawTool.title) || undefined,
    });
    return items;
  }, []);
  const rawServerInfo = isJSONRecord(response.server_info) ? response.server_info : null;
  const serverInfo = rawServerInfo
    ? {
        name: stringFromUnknown(rawServerInfo.name) || undefined,
        title: stringFromUnknown(rawServerInfo.title) || undefined,
        version: stringFromUnknown(rawServerInfo.version) || undefined,
      }
    : undefined;
  const duration = numberFromUnknown(response.duration_ms);
  return {
    connected: true,
    durationMs: duration === null ? 0 : Math.max(0, Math.trunc(duration)),
    protocolVersion: stringFromUnknown(response.protocol_version) || undefined,
    serverInfo,
    tools,
    toolsSupported: response.tools_supported === true,
    truncated: response.truncated === true,
  };
}

export function mcpToolParameters(tool: MCPProbeTool): MCPToolParameter[] {
  const schema = tool.inputSchema;
  const properties = schema && isJSONRecord(schema.properties) ? schema.properties : null;
  if (!properties) {
    return [];
  }
  const required = new Set(
    Array.isArray(schema?.required) ? schema.required.map((item) => stringFromUnknown(item)).filter(Boolean) : [],
  );
  return Object.entries(properties)
    .reduce<MCPToolParameter[]>((items, [rawName, rawSchema]) => {
      const name = rawName.trim();
      if (!name || !isJSONRecord(rawSchema)) {
        return items;
      }
      const rawType = rawSchema.type;
      const type = Array.isArray(rawType)
        ? rawType
            .map((item) => stringFromUnknown(item))
            .filter(Boolean)
            .join(" | ")
        : stringFromUnknown(rawType);
      items.push({
        description: stringFromUnknown(rawSchema.description) || undefined,
        name,
        required: required.has(name),
        type: type || "any",
      });
      return items;
    }, [])
    .sort((left, right) => Number(right.required) - Number(left.required) || left.name.localeCompare(right.name));
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

function stringFromUnknown(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function numberFromUnknown(value: unknown): number | null {
  const number = typeof value === "string" ? Number(value) : value;
  return typeof number === "number" && Number.isFinite(number) ? number : null;
}

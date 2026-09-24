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

export type MCPManagedKnowledgeBaseSource = {
  contentID: string;
  resourceID: string;
};

export type MCPServerSourceStatus = {
  agentUpdateAvailable?: boolean;
  authType: string;
  configuredEndpointURL: string;
  contentID: string;
  globalServerName?: string;
  kind: string;
  latestEndpointURL: string;
  resourceID: string;
  sourceAvailable: boolean;
  sourceDescription?: string;
  sourceName?: string;
  updateAvailable: boolean;
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
    .sort((left, right) => mcpServerDisplayName(left).localeCompare(mcpServerDisplayName(right)));
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

// `name` is the persisted MCP identity; display_name is editable presentation.
export function mcpServerDisplayName(server: MCPServer): string {
  const label = server.config.display_name;
  return typeof label === "string" && label.trim() ? label.trim() : server.name;
}

export function isRemoteMCPInstalled(servers: readonly MCPServer[], remote: RemoteMCPServer): boolean {
  return servers.some((server) => {
    const meta = isJSONRecord(server.config._meta) ? server.config._meta : null;
    const source = meta && isJSONRecord(meta["com.opencsg/marketplace"]) ? meta["com.opencsg/marketplace"] : null;
    return source ? source.server_id === remote.id : mcpServerDisplayName(server) === remote.name;
  });
}

export function hasMCPServerName(servers: readonly MCPServer[], name: string | null | undefined): boolean {
  const normalizedName = String(name || "").trim();
  return Boolean(normalizedName && servers.some((server) => server.name === normalizedName));
}

export function mcpManagedKnowledgeBaseSource(
  config: JSONRecord | null | undefined,
): MCPManagedKnowledgeBaseSource | null {
  if (!config) {
    return null;
  }
  const meta = isJSONRecord(config._meta) ? config._meta : null;
  const managed = meta && isJSONRecord(meta["com.opencsg/mcp"]) ? meta["com.opencsg/mcp"] : null;
  if (managed && stringFromUnknown(managed.type) === "llm_wiki") {
    const resourceID = stringFromUnknown(managed.resource_id);
    const contentID = stringFromUnknown(managed.content_id);
    if (resourceID && contentID && stringFromUnknown(managed.auth_type) === "csghub_access_token") {
      return { contentID, resourceID };
    }
  }
  return null;
}

export function managedMCPServerSnapshotDiffers(
  current: JSONRecord | null | undefined,
  candidate: JSONRecord | null | undefined,
): boolean {
  const currentSource = mcpManagedKnowledgeBaseSource(current);
  const candidateSource = mcpManagedKnowledgeBaseSource(candidate);
  if (
    !currentSource ||
    !candidateSource ||
    currentSource.resourceID !== candidateSource.resourceID ||
    currentSource.contentID !== candidateSource.contentID
  ) {
    return false;
  }
  return ["type", "url", "transport", "headers"].some(
    (key) =>
      JSON.stringify(comparableJSONValue(current?.[key])) !== JSON.stringify(comparableJSONValue(candidate?.[key])),
  );
}

export function mcpServerPayloadFromDocument(document: unknown): MCPServerPayload | null {
  const entries = Object.entries(cloneMCPServersRecord(mcpServerMapFromCatalogResponse(document)));
  if (entries.length !== 1) {
    return null;
  }
  const [name, serverConfig] = entries[0];
  return {
    name: typeof serverConfig.display_name === "string" ? serverConfig.display_name.trim() : name,
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

function comparableJSONValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(comparableJSONValue);
  }
  if (!isJSONRecord(value)) {
    return value;
  }
  return Object.fromEntries(
    Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, nested]) => [key, comparableJSONValue(nested)]),
  );
}

function stringFromUnknown(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function numberFromUnknown(value: unknown): number | null {
  const number = typeof value === "string" ? Number(value) : value;
  return typeof number === "number" && Number.isFinite(number) ? number : null;
}

export function mcpServerDetailConfig(config: JSONRecord): JSONRecord {
  const output: JSONRecord = {};
  for (const key of ["command", "transport", "startup_timeout_sec", "tool_timeout_sec", "enabled"]) {
    if (config[key] !== undefined) output[key] = config[key];
  }
  if (typeof config.url === "string") {
    try {
      const url = new URL(config.url);
      url.username = "";
      url.password = "";
      url.search = "";
      url.hash = "";
      output.url = url.toString();
    } catch {
      output.url = "[redacted]";
    }
  }
  for (const key of ["env", "headers", "http_headers"]) {
    const value = config[key];
    if (isJSONRecord(value)) output[key] = Object.fromEntries(Object.keys(value).map((name) => [name, "••••••"]));
  }
  // 参数可以携带认证信息，详情只展示参数数量。
  if (Array.isArray(config.args)) output.argument_count = config.args.length;
  return output;
}

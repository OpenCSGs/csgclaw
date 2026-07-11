export const AgentRuntimeStatuses = {
  installed: "installed",
  notInstalled: "not_installed",
  installing: "installing",
  failed: "failed",
  comingSoon: "coming_soon",
  unsupported: "unsupported",
} as const;

export type AgentRuntimeStatus = (typeof AgentRuntimeStatuses)[keyof typeof AgentRuntimeStatuses];

export type AgentRuntime = {
  name: string;
  label: string;
  supported: boolean;
  installed: boolean;
  installable: boolean;
  status: AgentRuntimeStatus;
  path?: string;
  os?: string;
  arch?: string;
  docsURL?: string;
  message?: string;
};

const runtimeOrder = new Map([
  ["codex", 0],
  ["claude_code", 1],
]);

const knownStatuses = new Set<AgentRuntimeStatus>(Object.values(AgentRuntimeStatuses));

export function normalizeAgentRuntimeName(value: unknown): string {
  return String(value ?? "")
    .trim()
    .toLowerCase()
    .replaceAll("-", "_");
}

export function normalizeAgentRuntime(value: unknown): AgentRuntime | null {
  const record = value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
  const name = normalizeAgentRuntimeName(record.name);
  if (!name) {
    return null;
  }

  const installed = Boolean(record.installed);
  const supported = typeof record.supported === "boolean" ? record.supported : name === "codex";
  const rawStatus = String(record.status ?? "").trim() as AgentRuntimeStatus;
  const status = installed
    ? AgentRuntimeStatuses.installed
    : knownStatuses.has(rawStatus)
      ? rawStatus
      : !supported
        ? AgentRuntimeStatuses.comingSoon
        : AgentRuntimeStatuses.notInstalled;

  return {
    name,
    label: stringValue(record.label) || fallbackRuntimeLabel(name),
    supported,
    installed: installed || status === AgentRuntimeStatuses.installed,
    installable: Boolean(record.installable),
    status,
    path: stringValue(record.path) || undefined,
    os: stringValue(record.os) || undefined,
    arch: stringValue(record.arch) || undefined,
    docsURL: stringValue(record.docs_url) || undefined,
    message: stringValue(record.message) || undefined,
  };
}

export function normalizeAgentRuntimeList(value: unknown): AgentRuntime[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const runtimes = new Map<string, AgentRuntime>();
  for (const item of value) {
    const runtime = normalizeAgentRuntime(item);
    if (runtime) {
      runtimes.set(runtime.name, runtime);
    }
  }
  return sortAgentRuntimes([...runtimes.values()]);
}

export function upsertAgentRuntime(runtimes: readonly AgentRuntime[], value: unknown): AgentRuntime[] {
  const runtime = normalizeAgentRuntime(value);
  if (!runtime) {
    return [...runtimes];
  }
  return sortAgentRuntimes([...runtimes.filter((item) => item.name !== runtime.name), runtime]);
}

export function agentRuntimeByName(
  runtimes: readonly AgentRuntime[] | null | undefined,
  name: string,
): AgentRuntime | null {
  const normalizedName = normalizeAgentRuntimeName(name);
  return runtimes?.find((runtime) => runtime.name === normalizedName) ?? null;
}

export function shouldPollAgentRuntimeInstallation(runtimes: readonly AgentRuntime[] | null | undefined): boolean {
  const codex = agentRuntimeByName(runtimes, "codex");
  return Boolean(
    codex && (codex.status === AgentRuntimeStatuses.notInstalled || codex.status === AgentRuntimeStatuses.installing),
  );
}

function sortAgentRuntimes(runtimes: AgentRuntime[]): AgentRuntime[] {
  return runtimes.sort((left, right) => {
    const leftRank = runtimeOrder.get(left.name) ?? Number.MAX_SAFE_INTEGER;
    const rightRank = runtimeOrder.get(right.name) ?? Number.MAX_SAFE_INTEGER;
    return leftRank - rightRank || left.label.localeCompare(right.label) || left.name.localeCompare(right.name);
  });
}

function fallbackRuntimeLabel(name: string): string {
  switch (name) {
    case "codex":
      return "Codex CLI";
    case "claude_code":
      return "Claude Code";
    default:
      return name;
  }
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

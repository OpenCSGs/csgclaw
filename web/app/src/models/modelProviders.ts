import type { ProviderName } from "@/models/agents";
import {
  modelProviderPresetMeta,
  normalizeModelProviderPreset,
  type ModelProviderPreset,
} from "@/models/modelProviderPresets";

export const BUILTIN_MODEL_PROVIDER_IDS = ["opencsg", "csghub-lite", "codex", "claude_code"] as const;
const MODEL_PROVIDER_AVATARS: Record<string, string> = {
  opencsg: "model-providers/opencsg.svg",
  "csghub-lite": "model-providers/csghub-lite.png",
  codex: "model-providers/codex.svg",
  claude_code: "model-providers/claude-code.svg",
  openai: "model-providers/openai-api.svg",
  zhipu: "model-providers/zhipu.svg",
  deepseek: "model-providers/deepseek.svg",
  custom: "model-providers/customize.svg",
};

const builtinRank = new Map<string, number>(BUILTIN_MODEL_PROVIDER_IDS.map((id, index) => [id, index]));

export type ModelProviderStatus = "unknown" | "connected" | "failed" | string;

export type ModelProvider = {
  id: string;
  kind: string;
  display_name: string;
  preset: ModelProviderPreset;
  builtin: boolean;
  base_url?: string;
  api_key_set?: boolean;
  api_key_preview?: string;
  headers?: Record<string, unknown>;
  models: string[];
  reasoning_effort?: string;
  status: ModelProviderStatus;
  message?: string;
  last_checked_at?: string;
};

export type ModelProviderCatalog = {
  providers: ModelProvider[];
  builtinProviders: ModelProvider[];
  customProviders: ModelProvider[];
};

export type ModelProviderOption = {
  value: string;
  label: string;
  providerID: string;
  providerDisplayName: string;
  providerAvatar: string;
  modelID: string;
  builtin?: boolean;
};

export type ModelProviderSelectOption = {
  avatar: string;
  builtin?: boolean;
  displayName: string;
  id: string;
  models: string[];
  value: string;
};

type RawCatalog = {
  providers?: unknown;
};

export function normalizeModelProviderCatalog(raw: RawCatalog | null | undefined): ModelProviderCatalog {
  const providers = (Array.isArray(raw?.providers) ? raw.providers : [])
    .map(normalizeModelProvider)
    .filter((provider): provider is ModelProvider => Boolean(provider.id));
  providers.sort(compareModelProviders);
  return {
    providers,
    builtinProviders: providers.filter((provider) => provider.builtin),
    customProviders: providers.filter((provider) => !provider.builtin),
  };
}

function normalizeModelProvider(raw: unknown): ModelProvider {
  const record = raw && typeof raw === "object" && !Array.isArray(raw) ? (raw as Record<string, unknown>) : {};
  const id = normalizeModelProviderID(record.id);
  const kind = String(record.kind ?? "").trim();
  const displayName = String(record.display_name ?? record.displayName ?? id).trim() || id;
  const models = normalizeModelIDs(record.models);
  return {
    id,
    kind,
    display_name: displayName,
    preset: normalizeModelProviderPreset(record.preset ?? inferModelProviderPreset(id, record.base_url)),
    builtin: Boolean(record.builtin) || builtinRank.has(id),
    base_url: String(record.base_url ?? "").trim() || undefined,
    api_key_set: Boolean(record.api_key_set),
    api_key_preview: String(record.api_key_preview ?? "").trim() || undefined,
    headers:
      record.headers && typeof record.headers === "object" && !Array.isArray(record.headers)
        ? (record.headers as Record<string, unknown>)
        : undefined,
    models,
    reasoning_effort: String(record.reasoning_effort ?? "").trim() || undefined,
    status: String(record.status ?? "unknown").trim() || "unknown",
    message: String(record.message ?? "").trim() || undefined,
    last_checked_at: String(record.last_checked_at ?? "").trim() || undefined,
  };
}

export function normalizeModelProviderID(value: unknown): string {
  const raw = String(value ?? "")
    .trim()
    .toLowerCase();
  if (!raw) {
    return "";
  }
  if (raw === "opencsg" || raw === "open-csg" || raw === "csghub") {
    return "opencsg";
  }
  if (raw === "csghub_lite" || raw === "csghublite") {
    return "csghub-lite";
  }
  if (raw === "claude-code" || raw === "claude") {
    return "claude_code";
  }
  const normalized = Array.from(raw)
    .map((char) => {
      if (/[\p{L}\p{N}]/u.test(char)) {
        return char;
      }
      if (char === "_" || char === "-") {
        return char;
      }
      if (/[\s./:]/u.test(char)) {
        return "-";
      }
      return "-";
    })
    .join("")
    .replace(/-{2,}/g, "-")
    .replace(/^[-_]+|[-_]+$/g, "");
  return normalized;
}

export function providerIDForProvider(provider: ProviderName | null | undefined): string {
  switch (String(provider ?? "").trim()) {
    case "csghub":
      return "opencsg";
    case "csghub_lite":
      return "csghub-lite";
    case "codex":
      return "codex";
    case "claude_code":
      return "claude_code";
    case "api":
      return "";
    default:
      return "";
  }
}

export function providerNameForProviderID(providerID: string): ProviderName {
  switch (normalizeModelProviderID(providerID)) {
    case "opencsg":
      return "csghub";
    case "csghub-lite":
      return "csghub_lite";
    case "codex":
      return "codex";
    case "claude_code":
      return "claude_code";
    default:
      return "api";
  }
}

export function selectorForProviderModel(providerID: string, modelID: string): string {
  const provider = normalizeModelProviderID(providerID);
  const model = String(modelID ?? "").trim();
  return provider && model ? `${provider}.${model}` : "";
}

export function splitProviderModelSelector(selector: unknown): { providerID: string; modelID: string } | null {
  const value = String(selector ?? "").trim();
  if (!value) {
    return null;
  }
  const dot = value.indexOf(".");
  const colon = value.indexOf(":");
  const splitAt = dot === -1 ? colon : colon === -1 ? dot : Math.min(dot, colon);
  if (splitAt <= 0) {
    return null;
  }
  const providerID = normalizeModelProviderID(value.slice(0, splitAt));
  const modelID = value.slice(splitAt + 1).trim();
  return providerID && modelID ? { providerID, modelID } : null;
}

export function modelProviderOptionsFromCatalog(
  catalog: ModelProviderCatalog | null | undefined,
): ModelProviderOption[] {
  const options: ModelProviderOption[] = [];
  const seen = new Set<string>();
  for (const provider of catalog?.providers ?? []) {
    const providerAvatar = modelProviderAvatarPath(provider);
    for (const modelID of provider.models) {
      const value = selectorForProviderModel(provider.id, modelID);
      if (!value || seen.has(value)) {
        continue;
      }
      seen.add(value);
      options.push({
        value,
        label: `${provider.display_name} / ${modelID}`,
        providerID: provider.id,
        providerDisplayName: provider.display_name,
        providerAvatar,
        modelID,
        builtin: provider.builtin || undefined,
      });
    }
  }
  return options;
}

export function modelProviderSelectOptionsFromCatalog(
  catalog: ModelProviderCatalog | null | undefined,
  modelOptions: readonly ModelProviderOption[] = [],
): ModelProviderSelectOption[] {
  const mergeModelOption = (providers: Map<string, ModelProviderSelectOption>, option: ModelProviderOption) => {
    const providerID = normalizeModelProviderID(option.providerID);
    if (!providerID) {
      return;
    }
    const existing = providers.get(providerID);
    if (existing) {
      if (option.modelID && !existing.models.includes(option.modelID)) {
        existing.models.push(option.modelID);
      }
      return;
    }
    providers.set(providerID, {
      avatar: option.providerAvatar || modelProviderAvatarPath(providerID),
      builtin: option.builtin,
      displayName: option.providerDisplayName || providerID,
      id: providerID,
      models: option.modelID ? [option.modelID] : [],
      value: providerID,
    });
  };

  if (catalog?.providers?.length) {
    const providers = new Map<string, ModelProviderSelectOption>();
    for (const provider of catalog.providers) {
      providers.set(provider.id, {
        avatar: modelProviderAvatarPath(provider),
        builtin: provider.builtin || undefined,
        displayName: provider.display_name || provider.id,
        id: provider.id,
        models: [...provider.models],
        value: provider.id,
      });
    }
    for (const option of modelOptions) {
      mergeModelOption(providers, option);
    }
    return [...providers.values()];
  }

  const providers = new Map<string, ModelProviderSelectOption>();
  for (const option of modelOptions) {
    mergeModelOption(providers, option);
  }
  return [...providers.values()];
}

export function modelProviderDisplayNameExists(
  catalog: ModelProviderCatalog | null | undefined,
  displayName: string,
  ignoreID = "",
): boolean {
  const normalizedName = normalizeDisplayName(displayName);
  const normalizedIgnoreID = normalizeModelProviderID(ignoreID);
  if (!normalizedName) {
    return false;
  }
  return (catalog?.providers ?? []).some(
    (provider) =>
      normalizeModelProviderID(provider.id) !== normalizedIgnoreID &&
      normalizeDisplayName(provider.display_name) === normalizedName,
  );
}

export function parseModelProviderModelsText(value: string): string[] {
  const seen = new Set<string>();
  const models: string[] = [];
  String(value ?? "")
    .split(/\r?\n|,/)
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((item) => {
      if (!seen.has(item)) {
        seen.add(item);
        models.push(item);
      }
    });
  return models;
}

export function modelProviderAvatarPath(provider: Pick<ModelProvider, "id" | "builtin" | "preset"> | string): string {
  if (typeof provider !== "string" && !provider.builtin) {
    const presetAvatar = modelProviderPresetMeta(provider.preset).avatar;
    if (presetAvatar) {
      return presetAvatar;
    }
  }
  const id = normalizeModelProviderID(typeof provider === "string" ? provider : provider.id);
  return MODEL_PROVIDER_AVATARS[id] || MODEL_PROVIDER_AVATARS.openai;
}

export function providerStatusTone(
  status: ModelProviderStatus | null | undefined,
  provider?: Partial<Pick<ModelProvider, "builtin" | "id">> | null,
): "online" | "warning" | "neutral" {
  switch (String(status ?? "").trim()) {
    case "connected":
      return "online";
    case "failed":
      return "warning";
    default:
      if (normalizeModelProviderID(provider?.id) === "opencsg") {
        return "warning";
      }
      return provider?.builtin ? "online" : "neutral";
  }
}

function normalizeDisplayName(value: unknown): string {
  return String(value ?? "")
    .trim()
    .replace(/\s+/g, " ")
    .toLowerCase();
}

function compareModelProviders(a: ModelProvider, b: ModelProvider): number {
  const rankA = builtinRank.get(a.id);
  const rankB = builtinRank.get(b.id);
  if (rankA !== undefined || rankB !== undefined) {
    return (rankA ?? 1000) - (rankB ?? 1000);
  }
  return a.display_name.localeCompare(b.display_name) || a.id.localeCompare(b.id);
}

function normalizeModelIDs(raw: unknown): string[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  const seen = new Set<string>();
  const models: string[] = [];
  for (const item of raw) {
    const modelID = String(item ?? "").trim();
    if (!modelID || seen.has(modelID)) {
      continue;
    }
    seen.add(modelID);
    models.push(modelID);
  }
  return models;
}

function inferModelProviderPreset(id: unknown, baseURL: unknown): ModelProviderPreset {
  const normalizedID = normalizeModelProviderID(id);
  if (normalizedID === "openai") {
    return "openai";
  }
  const url = String(baseURL ?? "")
    .trim()
    .toLowerCase();
  if (url.includes("bigmodel.cn") || url.includes("zhipu")) {
    return "zhipu";
  }
  if (url.includes("deepseek.com")) {
    return "deepseek";
  }
  return "custom";
}

export type ModelProviderPreset = string;
export type ModelAPIProtocol = "responses" | "chat_completions" | "anthropic_messages";
export type ModelProviderRegion = {
  id: string;
  name: string;
  base_url: string;
  key_url: string;
  protocol_urls?: Partial<Record<ModelAPIProtocol, string>>;
  default_protocol?: ModelAPIProtocol;
};
export type ModelServicePreset = {
  id: string;
  brand: string;
  brand_name: string;
  name: string;
  avatar: string;
  aliases: string[];
  api_protocol: ModelAPIProtocol;
  docs_url: string;
  regions: ModelProviderRegion[];
  models: string[];
};
export function normalizeModelProviderPreset(value: unknown): string {
  return (
    String(value ?? "")
      .trim()
      .toLowerCase() || "custom"
  );
}

const protocolOptions = [
  { value: "responses", label: "OpenAI Responses" },
  { value: "chat_completions", label: "OpenAI Chat Completions" },
  { value: "anthropic_messages", label: "Anthropic Messages" },
] as const;

export function usesModelProviderPresetURL(region: ModelProviderRegion | undefined, baseURL: string) {
  if (!region?.protocol_urls) return false;
  const normalized = baseURL.trim().replace(/\/+$/, "").toLowerCase();
  return (
    !normalized ||
    [region.base_url, ...Object.values(region.protocol_urls)].some((url) => url?.toLowerCase() === normalized)
  );
}

export function modelProviderProtocolOptions(region: ModelProviderRegion | undefined, baseURL: string) {
  const available = usesModelProviderPresetURL(region, baseURL) ? region?.protocol_urls : undefined;
  return protocolOptions.map((option) => ({ ...option, disabled: Boolean(available && !available[option.value]) }));
}

export function modelProviderProtocolURL(region: ModelProviderRegion | undefined, protocol: string) {
  return region?.protocol_urls?.[protocol as ModelAPIProtocol];
}

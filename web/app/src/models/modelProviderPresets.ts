export type ModelProviderPreset = string;
export type ModelAPIProtocol = "responses" | "chat_completions" | "anthropic_messages";
export type ModelProviderRegion = { id: string; name: string; base_url: string; key_url: string };
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

export const MODEL_PROVIDER_PRESETS = {
  openai: {
    avatar: "model-providers/openai-api.svg",
    defaultBaseURL: "https://api.openai.com/v1",
    defaultDisplayName: "OpenAI API",
  },
  zhipu: {
    avatar: "model-providers/zhipu.svg",
    defaultBaseURL: "https://open.bigmodel.cn/api/paas/v4",
    defaultDisplayName: "Zhipu API",
  },
  deepseek: {
    avatar: "model-providers/deepseek.svg",
    defaultBaseURL: "https://api.deepseek.com/v1",
    defaultDisplayName: "DeepSeek API",
  },
  custom: {
    avatar: "model-providers/customize.svg",
    defaultBaseURL: "",
    defaultDisplayName: "Custom API",
  },
} as const;

export type ModelProviderPreset = keyof typeof MODEL_PROVIDER_PRESETS;

export function normalizeModelProviderPreset(value: unknown): ModelProviderPreset {
  switch (
    String(value ?? "")
      .trim()
      .toLowerCase()
  ) {
    case "openai":
      return "openai";
    case "zhipu":
      return "zhipu";
    case "deepseek":
      return "deepseek";
    default:
      return "custom";
  }
}

export function modelProviderPresetMeta(value: unknown) {
  return MODEL_PROVIDER_PRESETS[normalizeModelProviderPreset(value)];
}

import {
  modelProviderDisplayNameExists,
  modelProviderOptionsFromCatalog,
  modelProviderSelectOptionsFromCatalog,
  normalizeModelProviderCatalog,
  parseModelProviderModelsText,
  providerStatusTone,
} from "@/models/modelProviders";

describe("model provider catalog helpers", () => {
  it("normalizes builtins before custom OpenAI-compatible providers", () => {
    const catalog = normalizeModelProviderCatalog({
      providers: [
        {
          id: "openai",
          kind: "openai_compatible",
          display_name: "Team OpenAI",
          api_key_set: true,
          models: ["gpt-4.1"],
        },
        {
          id: "codex",
          kind: "codex",
          builtin: true,
          display_name: "Codex",
          models: ["gpt-5.5"],
          status: "connected",
        },
        {
          id: "csghub-lite",
          kind: "csghub_lite",
          builtin: true,
          display_name: "CSGHub Lite",
          base_url: "http://127.0.0.1:11435/v1",
          models: ["qwen3"],
        },
        {
          id: "claude_code",
          kind: "claude_code",
          builtin: true,
          display_name: "Claude Code",
          models: ["claude-sonnet"],
        },
      ],
    });

    expect(catalog.providers.map((provider) => provider.id)).toEqual(["csghub-lite", "codex", "claude_code", "openai"]);
    expect(catalog.builtinProviders.map((provider) => provider.id)).toEqual(["csghub-lite", "codex", "claude_code"]);
    expect(catalog.customProviders.map((provider) => provider.id)).toEqual(["openai"]);
  });

  it("creates grouped model options from catalog providers", () => {
    const catalog = normalizeModelProviderCatalog({
      providers: [
        {
          id: "codex",
          kind: "codex",
          builtin: true,
          display_name: "Codex",
          models: ["gpt-5.5"],
        },
      ],
    });

    expect(modelProviderOptionsFromCatalog(catalog)).toEqual([
      {
        value: "codex.gpt-5.5",
        label: "Codex / gpt-5.5",
        providerID: "codex",
        providerDisplayName: "Codex",
        providerAvatar: "model-providers/codex.png",
        modelID: "gpt-5.5",
        builtin: true,
      },
    ]);
  });

  it("maps provider statuses to sidebar dot tones", () => {
    expect(providerStatusTone("connected")).toBe("online");
    expect(providerStatusTone("failed")).toBe("warning");
    expect(providerStatusTone("unknown")).toBe("neutral");
    expect(providerStatusTone("unknown", { builtin: true })).toBe("online");
  });

  it("detects duplicate provider display names", () => {
    const catalog = normalizeModelProviderCatalog({
      providers: [
        { id: "codex", display_name: "Codex", builtin: true, models: [] },
        { id: "openai", display_name: "OpenAI API", models: [] },
      ],
    });

    expect(modelProviderDisplayNameExists(catalog, " openai   api ")).toBe(true);
    expect(modelProviderDisplayNameExists(catalog, "Codex")).toBe(true);
  });

  it("parses model text from comma and newline separated input", () => {
    expect(parseModelProviderModelsText("gpt-4.1, gpt-4.1\n gpt-4.1-mini \n\nqwen3")).toEqual([
      "gpt-4.1",
      "gpt-4.1-mini",
      "qwen3",
    ]);
  });

  it("merges discovered model options into catalog providers that have no persisted models yet", () => {
    const catalog = normalizeModelProviderCatalog({
      providers: [{ id: "codex", display_name: "Codex", builtin: true, models: [] }],
    });

    const options = modelProviderSelectOptionsFromCatalog(catalog, [
      {
        value: "codex.gpt-5.5",
        label: "Codex / gpt-5.5",
        providerID: "codex",
        providerDisplayName: "Codex",
        providerAvatar: "model-providers/codex.png",
        modelID: "gpt-5.5",
        builtin: true,
      },
    ]);

    expect(options).toMatchObject([{ id: "codex", displayName: "Codex", models: ["gpt-5.5"] }]);
  });
});

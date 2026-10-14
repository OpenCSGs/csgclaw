import { describe, expect, it } from "vitest";
import {
  defaultModelIDForProvider,
  isOpenCSGAutoModel,
  mergeModelProviderModelIDs,
  modelIDsForProviderSelection,
} from "./modelProviders";

describe("mergeModelProviderModelIDs", () => {
  it("includes video-only models and keeps generative models after chat models", () => {
    expect(
      mergeModelProviderModelIDs(
        ["chat-model", "shared-model"],
        ["image-model", "shared-model"],
        ["video-model", "shared-model"],
      ),
    ).toEqual(["chat-model", "shared-model", "image-model", "video-model"]);
  });
});

describe("OpenCSG auto model selection", () => {
  it("puts auto first and selects it by default without changing its model ID", () => {
    const models = modelIDsForProviderSelection("opencsg", ["qwen3", "auto", "deepseek"]);
    expect(models).toEqual(["auto", "qwen3", "deepseek"]);
    expect(defaultModelIDForProvider("opencsg", models)).toBe("auto");
    expect(isOpenCSGAutoModel("opencsg", models[0])).toBe(true);
  });

  it("keeps other providers in their original order", () => {
    const models = ["qwen3", "auto", "deepseek"];
    expect(modelIDsForProviderSelection("custom", models)).toEqual(models);
    expect(defaultModelIDForProvider("custom", models)).toBe("qwen3");
  });
});

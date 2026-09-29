import { describe, expect, it } from "vitest";
import { mergeModelProviderModelIDs } from "./modelProviders";

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

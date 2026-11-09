import { agentToDraft } from "@/models/agents";
import { normalizeModelProviderCatalog } from "@/models/modelProviders";
import { agentDraftWithAvailableModels } from "./agentCreationModels";

const catalog = normalizeModelProviderCatalog({
  providers: [
    {
      id: "failed",
      status: "failed",
      models: ["stale-chat"],
      image_models: ["stale-image"],
      video_models: ["stale-video"],
    },
    {
      id: "service",
      status: "connected",
      models: ["chat-first", "image-first", "video-first", "chat-last"],
      image_models: ["image-first", "image-last"],
      video_models: ["video-first", "video-last"],
    },
    {
      id: "other",
      status: "connected",
      models: ["other-chat"],
      image_models: ["other-image"],
      video_models: ["other-video"],
    },
  ],
});

it("selects available conversation, image and video models without a saved selection", () => {
  expect(agentDraftWithAvailableModels(agentToDraft({}), catalog)).toMatchObject({
    model_provider_id: "other",
    model_id: "other-chat",
    image_generation: { provider_id: "other", model_id: "other-image" },
    video_generation: { provider_id: "other", model_id: "other-video" },
  });
});
it("keeps all three remembered selections even when another service comes first", () => {
  const draft = agentToDraft({
    agent_profile: {
      model_provider_id: "other",
      model_id: "other-chat",
      image_generation: { provider_id: "service", model_id: "image-last" },
      video_generation: { provider_id: "service", model_id: "video-last" },
    },
  });
  expect(agentDraftWithAvailableModels(draft, catalog)).toMatchObject({
    model_provider_id: "other",
    model_id: "other-chat",
    image_generation: draft.image_generation,
    video_generation: draft.video_generation,
  });
});
it("replaces unavailable choices independently and never selects media as the chat model", () => {
  const draft = agentToDraft({
    agent_profile: {
      model_provider_id: "failed",
      model_id: "stale-chat",
      image_generation: { provider_id: "service", model_id: "image-gone" },
      video_generation: { provider_id: "other", model_id: "other-video" },
    },
  });
  expect(agentDraftWithAvailableModels(draft, catalog)).toMatchObject({
    model_provider_id: "other",
    model_id: "other-chat",
    image_generation: { provider_id: "other", model_id: "other-image" },
    video_generation: { provider_id: "other", model_id: "other-video" },
  });
});
it("leaves missing categories empty and excludes signed-out OpenCSG", () => {
  const providers = normalizeModelProviderCatalog({
    providers: [
      { id: "opencsg", status: "connected", models: ["auto"], image_models: ["image"], video_models: ["video"] },
    ],
  });
  expect(agentDraftWithAvailableModels(agentToDraft({}), providers, false)).toMatchObject({
    model_id: "",
    image_generation: null,
    video_generation: null,
  });
});
it("prefers the OpenCSG auto route when selecting its initial chat model", () => {
  const providers = normalizeModelProviderCatalog({
    providers: [{ id: "opencsg", status: "connected", models: ["other", "auto"] }],
  });
  expect(agentDraftWithAvailableModels(agentToDraft({}), providers)).toMatchObject({
    model_provider_id: "opencsg",
    model_id: "auto",
  });
});

import { effectiveConversationProfile } from "@/models/taskConfiguration";
import { normalizeModelProviderCatalog } from "@/models/modelProviders";

const agent = {
  model_config: { model_provider_id: "codex", model_id: "default", provider: "codex", profile_complete: false },
};

it("follows the Agent profile until a room model is selected", () => {
  expect(effectiveConversationProfile(agent, { reasoning_effort: "high" })).toBe(agent.model_config);
});

it("uses the overridden provider kind without retaining the Agent authentication or incomplete flag", () => {
  const providers = normalizeModelProviderCatalog({
    providers: [{ id: "local-server", kind: "openai", models: ["local-model"] }],
  });
  expect(
    effectiveConversationProfile(agent, { model_provider_id: "local-server", model_id: "local-model" }, providers),
  ).toMatchObject({
    provider: "openai",
    model_provider_id: "local-server",
    model_id: "local-model",
    profile_complete: true,
  });
  expect(agent.model_config.profile_complete).toBe(false);
});

it.each([{ model_provider_id: "codex" }, { model_id: "only-model" }])(
  "rejects an incomplete model/provider pair: %j",
  (configuration) => {
    expect(effectiveConversationProfile(agent, configuration)?.profile_complete).toBe(false);
  },
);
